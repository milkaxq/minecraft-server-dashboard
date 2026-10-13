package management

import (
	"context"
	"path"
	"strings"
	"time"

	"github.com/google/uuid"

	"minecraft-hosting/internal/control"
	"minecraft-hosting/internal/server"
)

type StorageCategory struct {
	Name  string `json:"name"`
	Bytes int64  `json:"bytes"`
}
type Storage struct {
	UsedBytes   int64             `json:"used_bytes"`
	LimitBytes  int64             `json:"limit_bytes"`
	BackupBytes int64             `json:"backup_bytes"`
	BackupCount int               `json:"backup_count"`
	BackupLimit int               `json:"backup_limit"`
	Categories  []StorageCategory `json:"categories"`
	Partial     bool              `json:"partial"`
	CheckedAt   time.Time         `json:"checked_at"`
}

// Storage walks only Panel-scoped paths, with strict work/time bounds. Breakdown
// is approximate while Minecraft writes files; Wings usage is the quota figure.
func (s *Service) Storage(ctx context.Context, user, id uuid.UUID) (Storage, error) {
	t, err := s.access(ctx, user, id, server.Files)
	if err != nil {
		return Storage{}, err
	}
	u, err := s.Control.Panel.Resources(ctx, t.Identifier)
	if err != nil {
		return Storage{}, control.ErrUnavailable
	}
	out := Storage{UsedBytes: u.DiskBytes, LimitBytes: int64(t.DiskMB) * 1048576, BackupLimit: t.BackupLimit, CheckedAt: time.Now().UTC(), Categories: []StorageCategory{}}
	world := "world"
	if content, e := s.read(ctx, t, "/server.properties"); e == nil {
		if lines, e := parseProperties(content.Content); e == nil {
			for _, line := range lines {
				if line.key == "level-name" && Name(line.value) == nil && !strings.HasPrefix(line.value, ".") {
					world = line.value
				}
			}
		} else {
			out.Partial = true
		}
	} else {
		out.Partial = true
	}

	backups, err := s.Panel.ListBackups(ctx, t.Identifier)
	if err != nil {
		return out, control.ErrUnavailable
	}
	for _, b := range backups {
		out.BackupCount++
		if b.Bytes > 0 {
			out.BackupBytes += b.Bytes
		}
	}
	scan, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	type folder struct{ path, category string }
	queue := []folder{{"/", "Other"}}
	sizes := map[string]int64{}
	count := 0
	folders := 0
	for len(queue) > 0 {
		if count >= 5000 || folders >= 200 || len(queue) > 200 || scan.Err() != nil {
			out.Partial = true
			break
		}
		folders++
		current := queue[0]
		queue = queue[1:]
		entries, e := s.Panel.ListFiles(scan, t.Identifier, current.path)
		if e != nil {
			out.Partial = true
			break
		}
		for _, entry := range entries {
			count++
			if count > 5000 {
				out.Partial = true
				break
			}
			if entry.IsSymlink {
				out.Partial = true
				continue
			}
			if entry.Name == "" || entry.Name == "." || entry.Name == ".." || strings.ContainsAny(entry.Name, "/\\") {
				out.Partial = true
				continue
			}
			file := path.Join(current.path, entry.Name)
			if _, e = Path(file, false); e != nil {
				out.Partial = true
				continue
			}
			category := current.category
			if current.path == "/" {
				switch {
				case entry.Name == "plugins" || entry.Name == "mods":
					category = "Addons"
				case entry.Name == world || entry.Name == world+"_nether" || entry.Name == world+"_the_end":
					category = "Worlds"
				case entry.Name == "logs":
					category = "Logs"
				}
			}
			if entry.IsFile {
				if entry.Size >= 0 {
					sizes[category] += entry.Size
				}
			} else {
				queue = append(queue, folder{file, category})
			}
		}
	}
	for _, name := range []string{"Worlds", "Addons", "Logs", "Other"} {
		out.Categories = append(out.Categories, StorageCategory{name, sizes[name]})
	}
	if _, err = s.Control.Access(ctx, t.q, user, id, server.Files); err != nil {
		return Storage{}, err
	}
	return out, nil
}
