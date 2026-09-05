package parser

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const evenerTranscriptSuffix = ".transcript.jsonl"

func newEvenerProviderFactory(def AgentDef) ProviderFactory {
	return evenerProviderFactory{NewSourceSetFactory(def, evenerProviderCapabilities(), func(cfg ProviderConfig) SourceSet {
		return evenerSourceSet{NewSingleFileSourceSet(def.Type, cfg.Roots,
			WithStreamingFileDiscovery(evenerDiscoverEach),
			WithFileWatchRoots(evenerWatchRoots),
			WithFileChangedPathClassifier(evenerClassifyPath),
			WithFileLookup(evenerFindFile),
			WithFileFingerprint(evenerFingerprintSource),
			WithFileParse(evenerFileParser(context.Background())),
		)}
	})}
}

type evenerSourceSet struct{ singleFileSourceSet }

func (s evenerSourceSet) Discover(ctx context.Context) ([]SourceRef, error) {
	return collectDiscoveredSources(ctx, s.DiscoverEach)
}

func (s evenerSourceSet) DiscoverEach(ctx context.Context, yield func(SourceRef) error) error {
	seen := make(map[string]bool)
	return s.singleFileSourceSet.DiscoverEach(ctx, func(source SourceRef) error {
		if seen[source.Key] {
			return nil
		}
		seen[source.Key] = true
		return yield(source)
	})
}

func evenerDiscoverEach(ctx context.Context, root string, yield func(singleFileMatch) error) error {
	// Limit discovery to session directories; API logs and other state trees
	// are neither session sources nor useful discovery work.
	dirs := []string{filepath.Join(root, "sessions")}
	if filepath.Base(root) == "sessions" {
		dirs = []string{root}
	}
	projects, err := os.ReadDir(filepath.Join(root, "projects"))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	for _, project := range projects {
		if project.IsDir() {
			dirs = append(dirs, filepath.Join(root, "projects", project.Name(), "sessions"))
		}
	}
	for _, dir := range dirs {
		if err := ctx.Err(); err != nil {
			return err
		}
		entries, err := os.ReadDir(dir)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				return err
			}
			if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || !strings.HasSuffix(entry.Name(), evenerTranscriptSuffix) {
				continue
			}
			if match, ok := evenerClassifyPath(root, filepath.Join(dir, entry.Name()), false); ok {
				if err := yield(match); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func evenerWatchRoots(roots []string) []WatchRoot {
	out := make([]WatchRoot, 0, len(roots))
	for _, root := range roots {
		out = append(out, WatchRoot{Path: root, Recursive: true, IncludeGlobs: []string{"*.transcript.jsonl", "*.meta.json"}, DebounceKey: "evener:" + root})
	}
	return out
}

func evenerMetadataPath(path string) string {
	return strings.TrimSuffix(path, evenerTranscriptSuffix) + ".meta.json"
}

func evenerSafeID(id string) bool {
	return isSafeSinglePathComponent(id) && !strings.ContainsAny(id, "\\:\x00")
}

func evenerClassifyPath(root, path string, allowMissing bool) (singleFileMatch, bool) {
	root, path = filepath.Clean(root), filepath.Clean(path)
	rel, ok := relUnder(root, path)
	if !ok {
		return singleFileMatch{}, false
	}
	if stem, ok := strings.CutSuffix(path, ".meta.json"); ok {
		path = stem + evenerTranscriptSuffix
		rel = strings.TrimSuffix(rel, ".meta.json") + evenerTranscriptSuffix
	}
	id, ok := strings.CutSuffix(filepath.Base(path), evenerTranscriptSuffix)
	if !ok {
		return singleFileMatch{}, false
	}
	if !evenerSafeID(id) {
		return singleFileMatch{}, false
	}
	parts := strings.Split(rel, string(filepath.Separator))
	valid := len(parts) == 4 && parts[0] == "projects" && parts[2] == "sessions" ||
		len(parts) == 2 && parts[0] == "sessions" || len(parts) == 1 && filepath.Base(root) == "sessions"
	if !valid || (!allowMissing && !IsRegularFile(path)) {
		return singleFileMatch{}, false
	}
	// A valid lexical layout must not follow a symlink into another source root.
	resolvedRoot, rootErr := filepath.EvalSymlinks(root)
	resolvedDir, dirErr := filepath.EvalSymlinks(filepath.Dir(path))
	if rootErr == nil && dirErr == nil && resolvedRoot != resolvedDir && !pathUnderRoot(resolvedRoot, resolvedDir) {
		return singleFileMatch{}, false
	}
	if info, err := os.Lstat(path); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return singleFileMatch{}, false
	}
	return singleFileMatch{Path: path}, true
}

func evenerFindFile(root, id string) (singleFileMatch, bool) {
	if !evenerSafeID(id) {
		return singleFileMatch{}, false
	}
	var found singleFileMatch
	// The lookup reads directory names only; it never parses unrelated sessions.
	_ = evenerDiscoverEach(context.Background(), root, func(match singleFileMatch) error {
		if strings.TrimSuffix(filepath.Base(match.Path), evenerTranscriptSuffix) == id {
			found = match
		}
		return nil
	})
	return found, found.Path != ""
}

func (s evenerSourceSet) FindSource(ctx context.Context, req FindSourceRequest) (SourceRef, bool, error) {
	if req.RawSessionID != "" && !evenerSafeID(req.RawSessionID) {
		return SourceRef{}, false, nil
	}
	// Stored paths are hints, not permission to return another session's file.
	for _, hint := range []*string{&req.StoredFilePath, &req.FingerprintKey} {
		if *hint != "" && req.RawSessionID != "" && strings.TrimSuffix(filepath.Base(*hint), evenerTranscriptSuffix) != req.RawSessionID {
			*hint = ""
		}
	}
	return s.singleFileSourceSet.FindSource(ctx, req)
}

func (s evenerSourceSet) ComputeMultiFileStatHash(path string) uint64 {
	meta := evenerMetadataPath(path)
	if info, err := os.Lstat(meta); err == nil && !info.Mode().IsRegular() {
		return 0
	}
	parent, err := evenerParentTranscriptPath(path)
	if err != nil {
		return 0
	}
	if evenerParentFileInfo(parent) == nil {
		parent = ""
	}
	return fileStatTupleDigest(0xE7, path, meta, parent)
}

// evenerParentFileInfo excludes symlinks before either freshness path reads
// an optional parent; unverifiable parents leave the child's history intact.
func evenerParentFileInfo(path string) os.FileInfo {
	if path == "" {
		return nil
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return nil
	}
	return info
}

func evenerFingerprintSource(src singleFileSource) (SourceFingerprint, error) {
	info, err := os.Lstat(src.Path)
	if err != nil {
		return SourceFingerprint{}, err
	}
	if !info.Mode().IsRegular() {
		return SourceFingerprint{}, fmt.Errorf("evener transcript is not a regular file")
	}
	// The warm stat-digest gate uses the primary transcript's mtime. Metadata
	// freshness is represented by the per-file digest and required content hash.
	fp := SourceFingerprint{Size: info.Size(), MTimeNS: info.ModTime().UnixNano()}
	h := sha256.New()
	if err := addSiblingMetadataFingerprintPart(h, "transcript", src.Path, info); err != nil {
		return SourceFingerprint{}, err
	}
	meta := evenerMetadataPath(src.Path)
	mi, err := os.Lstat(meta)
	switch {
	case os.IsNotExist(err):
	case err != nil:
		return SourceFingerprint{}, err
	default:
		if !mi.Mode().IsRegular() {
			return SourceFingerprint{}, fmt.Errorf("evener metadata is not a regular file")
		}
		fp.Size += mi.Size()
		if err := addSiblingMetadataFingerprintPart(h, "metadata", meta, mi); err != nil {
			return SourceFingerprint{}, err
		}
	}
	// Fork prefix validation reads one immediate parent. Its arrival, removal,
	// or rewrite must invalidate the child without counting parent bytes as
	// part of the child raw source.
	parent, err := evenerParentTranscriptPath(src.Path)
	if err != nil {
		return SourceFingerprint{}, err
	}
	if parent != "" {
		info := evenerParentFileInfo(parent)
		if info == nil {
			_, _ = fmt.Fprintln(h, "parent:unavailable")
		} else if err := addSiblingMetadataFingerprintPart(h, "parent", parent, info); err != nil {
			// An optional parent that cannot be read cannot prove a copied
			// prefix. Retain child history and track future availability.
			_, _ = fmt.Fprintln(h, "parent:unavailable")
		}
	}
	fp.Hash = fmt.Sprintf("%x", h.Sum(nil))
	return fp, nil
}

func evenerFileParser(ctx context.Context) func(singleFileSource, ParseRequest) ([]ParseResult, []string, error) {
	return func(src singleFileSource, req ParseRequest) ([]ParseResult, []string, error) {
		sess, msgs, err := parseEvenerSession(ctx, src.Path, req.Machine)
		if err != nil || sess == nil {
			return nil, nil, err
		}
		// A full provider parse owns the source title, including metadata removal.
		sess.SessionNamePresent = true
		if req.Fingerprint.Hash != "" {
			sess.File.Size = req.Fingerprint.Size
			sess.File.Mtime = req.Fingerprint.MTimeNS
			sess.File.Hash = req.Fingerprint.Hash
		}
		return []ParseResult{{Session: *sess, Messages: msgs}}, nil, nil
	}
}

func (s evenerSourceSet) Parse(ctx context.Context, req ParseRequest) (ParseOutcome, error) {
	if req.Fingerprint.Hash == "" {
		fingerprint, err := s.Fingerprint(ctx, req.Source)
		if err != nil {
			return ParseOutcome{}, err
		}
		req.Fingerprint = fingerprint
	}
	// Bind the request context into the single-file parser callback.
	s.cfg.parseFile = evenerFileParser(ctx)
	out, err := s.singleFileSourceSet.Parse(ctx, req)
	if err != nil {
		return out, err
	}
	for _, result := range out.Results {
		if result.Result.Session.IsTruncated {
			// A writer may have replaced the file with a shorter unfinished
			// transcript. Do not replace archived history until it is framed.
			return ParseOutcome{ResultSetComplete: false}, nil
		}
	}
	out.ForceReplace = len(out.Results) > 0
	return out, nil
}

func evenerProviderCapabilities() Capabilities {
	caps := jsonlFileProviderSourceCapabilities()
	caps.CompositeFingerprint = CapabilitySupported
	caps.ForceReplaceOnParse = CapabilitySupported
	caps.MultiFileStatHash = CapabilitySupported
	return Capabilities{
		RawCapture: RawCaptureCapabilities{
			Support: CapabilitySupported,
			Shape:   RawCaptureShapeFiles,
			Append:  RawCaptureAppendReplaceOnly,
		},
		Source: caps,
		Content: ContentCapabilities{
			FirstMessage:         CapabilitySupported,
			SessionName:          CapabilitySupported,
			Cwd:                  CapabilitySupported,
			Thinking:             CapabilitySupported,
			ToolCalls:            CapabilitySupported,
			ToolResults:          CapabilitySupported,
			PerMessageTokenUsage: CapabilitySupported,
			Model:                CapabilitySupported,
			Relationships:        CapabilitySupported,
		},
		Sync: ProviderSyncSemantics{FingerprintHashRequiredForFreshness: true},
	}
}

// The provider adapter exposes physical companions to the existing raw capture
// pipeline. The source set remains responsible for normalized session parsing.
type evenerProviderFactory struct{ ProviderFactory }

func (f evenerProviderFactory) NewProvider(cfg ProviderConfig) Provider {
	return &evenerProvider{f.ProviderFactory.NewProvider(cfg).(*SourceSetProvider)}
}

type evenerProvider struct{ *SourceSetProvider }

func (p *evenerProvider) PlanRawCapture(ctx context.Context, source SourceRef) (RawCapturePlan, error) {
	if err := ctx.Err(); err != nil {
		return RawCapturePlan{}, err
	}
	src, ok := source.Opaque.(singleFileSource)
	if !ok || src.Root == "" || src.Path == "" || isS3URI(src.Root) {
		return RawCapturePlan{}, invalidRawCapturePlan("evener source is not a local discovered transcript")
	}
	if _, ok := evenerClassifyPath(src.Root, src.Path, false); !ok {
		return RawCapturePlan{}, invalidRawCapturePlan("evener source is outside its configured layout")
	}
	plan := RawCapturePlan{ConfiguredRoot: src.Root, CaptureRoot: src.Root, SourceKey: source.Key}
	for _, path := range []string{src.Path, evenerMetadataPath(src.Path)} {
		info, err := os.Lstat(path)
		if os.IsNotExist(err) && path != src.Path {
			continue
		}
		if err != nil {
			return RawCapturePlan{}, err
		}
		if !info.Mode().IsRegular() {
			return RawCapturePlan{}, invalidRawCapturePlan("evener capture source is not a regular file")
		}
		rel, err := filepath.Rel(src.Root, path)
		if err != nil {
			return RawCapturePlan{}, err
		}
		plan.Entries = append(plan.Entries, RawCaptureEntry{Path: filepath.ToSlash(rel), LocalPath: path})
	}
	return plan, nil
}
