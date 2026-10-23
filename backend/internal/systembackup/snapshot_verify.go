package systembackup

import (
	"archive/tar"
	"context"
	"errors"
	"io"
	"path"
	"strings"
)

// ValidatePublishedSnapshot adds format/coverage validation to authenticated
// decryption. It writes no restored data and needs no source-side connection.
// Successful ciphertext authentication alone cannot prove a usable snapshot.
func ValidatePublishedSnapshot(ctx context.Context, store ImmutableObjectStore, artifact PublishedArtifact, wrapper DataKeyWrapper, b CaptureBudget) error {
	if b.MaxBytes < 1<<20 || b.MaxItems < 1 || artifact.Manifest.Payload.Validate() != nil {
		return errors.New("validation_failed")
	}
	var total int64
	for _, p := range artifact.Manifest.Payload.Parts {
		if p.CiphertextBytes > b.MaxBytes-total {
			return errors.New("capacity_exceeded")
		}
		total += p.CiphertextBytes
		if err := ConsumePublishedPart(ctx, store, artifact, p.Name, wrapper, b.MaxBytes, func(r io.Reader) error { return ValidateSnapshotPart(ctx, p.Name, r, b) }); err != nil {
			return err
		}
	}
	return nil
}

func ValidateSnapshotPart(ctx context.Context, name string, r io.Reader, b CaptureBudget) error {
	if r == nil || b.MaxBytes < 1<<20 || b.MaxItems < 1 {
		return errors.New("validation_failed")
	}
	switch name {
	case "mysql":
		return ValidateBusinessSQLSnapshot(r)
	case "redis":
		return ReadRedisSnapshot(r, RedisBudget{MaxBytes: b.MaxBytes, MaxItems: b.MaxItems, MaxInventoryBytes: min(b.MaxBytes/8, 64<<20)}, nil)
	case "object_storage":
		return ReadObjectSnapshot(ctx, r, ObjectBudget{MaxBytes: b.MaxBytes, MaxItems: b.MaxItems}, nil)
	case "workspace":
		return ValidateFileSnapshot(ctx, r, FileBudget{MaxBytes: b.MaxBytes, MaxEntries: b.MaxItems})
	case "deployment_resources", "secret_bundle":
		return ReadKubernetesHistory(ctx, r, name, ObjectBudget{MaxBytes: b.MaxBytes, MaxItems: b.MaxItems}, nil)
	default:
		return errors.New("validation_failed")
	}
}

// ValidateFileSnapshot applies the restore archive rules without extracting
// plaintext. In particular, links can never turn a parent into a write escape.
func ValidateFileSnapshot(ctx context.Context, r io.Reader, b FileBudget) error {
	if err := b.validate(); err != nil {
		return err
	}
	tr := tar.NewReader(contextReader{ctx, r})
	seen := map[string]byte{}
	parents := map[string]bool{}
	var total int64
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if !safeArchiveName(h.Name) || len(seen) >= b.MaxEntries || h.Size < 0 || h.Size > b.MaxBytes-total || h.Uid < 0 || h.Gid < 0 || h.Mode & ^int64(0777) != 0 {
			return errors.New("validation_failed")
		}
		if _, ok := seen[h.Name]; ok {
			return errors.New("validation_failed")
		}
		for parent := path.Dir(h.Name); parent != "."; parent = path.Dir(parent) {
			if kind, ok := seen[parent]; ok && kind != tar.TypeDir {
				return errors.New("validation_failed")
			}
			parents[parent] = true
		}
		if parents[h.Name] && h.Typeflag != tar.TypeDir {
			return errors.New("validation_failed")
		}
		seen[h.Name] = h.Typeflag
		switch h.Typeflag {
		case tar.TypeDir:
			if h.Size != 0 {
				return errors.New("validation_failed")
			}
		case tar.TypeSymlink:
			if h.Size != 0 || strings.ContainsAny(h.Linkname, "\\:\x00") || path.IsAbs(h.Linkname) || !safeArchiveName(path.Join(path.Dir(h.Name), h.Linkname)) {
				return errors.New("validation_failed")
			}
		case tar.TypeReg:
			n, err := io.Copy(io.Discard, tr)
			if err != nil {
				return err
			}
			if n != h.Size {
				return io.ErrUnexpectedEOF
			}
			total += n
		default:
			return errors.New("validation_failed")
		}
	}
	var extra [1]byte
	if n, err := io.ReadFull(contextReader{ctx, r}, extra[:]); n != 0 || err != io.EOF {
		return errors.New("validation_failed")
	}
	return nil
}
