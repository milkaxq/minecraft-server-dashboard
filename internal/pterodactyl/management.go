package pterodactyl

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"time"

	"github.com/google/uuid"
)

const MaxTextBytes = 1 << 20
const MaxUploadBytes = 64 << 20

type FileEntry struct {
	Name       string `json:"name"`
	Size       int64  `json:"size"`
	IsFile     bool   `json:"is_file"`
	IsSymlink  bool   `json:"is_symlink"`
	ModifiedAt string `json:"modified_at"`
}
type Backup struct {
	UUID        string  `json:"uuid"`
	Name        string  `json:"name"`
	Successful  bool    `json:"is_successful"`
	Locked      bool    `json:"is_locked"`
	Bytes       int64   `json:"bytes"`
	CreatedAt   string  `json:"created_at"`
	CompletedAt *string `json:"completed_at"`
}

// ManagementClient exposes only container-scoped operations, never resources,
// host paths, shell commands or unrestricted URLs.
type ManagementClient interface {
	Ready(context.Context, string) error
	ListFiles(context.Context, string, string) ([]FileEntry, error)
	ReadFile(context.Context, string, string) ([]byte, error)
	WriteFile(context.Context, string, string, []byte) error
	CreateFolder(context.Context, string, string, string) error
	RenameFile(context.Context, string, string, string, string) error
	DeleteFile(context.Context, string, string, string) error
	UploadFile(context.Context, string, string, string, []byte) error
	DownloadFile(context.Context, string, string) (io.ReadCloser, error)
	ListBackups(context.Context, string) ([]Backup, error)
	CreateBackup(context.Context, string, string) (Backup, error)
	RestoreBackup(context.Context, string, string) error
	DeleteBackup(context.Context, string, string) error
	DownloadBackup(context.Context, string, string) (io.ReadCloser, error)
}

// ConfigureTransfers sets an explicit egress allowlist, including Wings and any
// backup object-store origins. Signed URLs and their credentials never reach users.
func (a *HTTPServerClient) ConfigureTransfers(origins []string) {
	a.transferOrigins = append([]string(nil), origins...)
}
func (a *HTTPServerClient) Ready(ctx context.Context, id string) error {
	if !safeIdentifier(id) {
		return ErrInvalid
	}
	var r struct {
		Attributes struct {
			Identifier   string  `json:"identifier"`
			Status       *string `json:"status"`
			Suspended    bool    `json:"is_suspended"`
			Installing   bool    `json:"is_installing"`
			Transferring bool    `json:"is_transferring"`
			Maintenance  bool    `json:"is_node_under_maintenance"`
		} `json:"attributes"`
	}
	if err := a.request(ctx, "GET", "/api/client/servers/"+id, nil, &r); err != nil {
		return err
	}
	v := r.Attributes
	if !safeIdentifier(v.Identifier) || v.Identifier != id {
		return ErrInvalid
	}
	if (v.Status != nil && *v.Status != "") || v.Suspended || v.Installing || v.Transferring || v.Maintenance {
		return ErrConflict
	}
	return nil
}
func prefix(id string) string { return "/api/client/servers/" + id }
func (a *HTTPServerClient) ListFiles(ctx context.Context, id, dir string) ([]FileEntry, error) {
	if !safeIdentifier(id) {
		return nil, ErrInvalid
	}
	var r struct {
		Data []struct {
			Attributes FileEntry `json:"attributes"`
		} `json:"data"`
	}
	if err := a.request(ctx, "GET", prefix(id)+"/files/list?directory="+url.QueryEscape(dir), nil, &r); err != nil {
		return nil, err
	}
	out := make([]FileEntry, 0, len(r.Data))
	for _, v := range r.Data {
		out = append(out, v.Attributes)
	}
	return out, nil
}
func (a *HTTPServerClient) ReadFile(ctx context.Context, id, file string) ([]byte, error) {
	if !safeIdentifier(id) {
		return nil, ErrInvalid
	}
	r, err := http.NewRequestWithContext(ctx, "GET", a.base+prefix(id)+"/files/contents?file="+url.QueryEscape(file), nil)
	if err != nil {
		return nil, ErrInvalid
	}
	r.Header.Set("Authorization", "Bearer "+a.key)
	response, err := a.http.Do(r)
	if err != nil {
		return nil, fmt.Errorf("panel read failed")
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return nil, &APIError{Status: response.StatusCode}
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, MaxTextBytes+1))
	if err != nil || len(data) > MaxTextBytes {
		return nil, ErrInvalid
	}
	return data, nil
}
func (a *HTTPServerClient) WriteFile(ctx context.Context, id, file string, data []byte) error {
	if !safeIdentifier(id) || len(data) > MaxUploadBytes {
		return ErrInvalid
	}
	r, err := http.NewRequestWithContext(ctx, "POST", a.base+prefix(id)+"/files/write?file="+url.QueryEscape(file), bytes.NewReader(data))
	if err != nil {
		return ErrInvalid
	}
	r.Header.Set("Authorization", "Bearer "+a.key)
	r.Header.Set("Content-Type", "application/octet-stream")
	response, err := a.http.Do(r)
	if err != nil {
		return fmt.Errorf("panel write failed")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return &APIError{Status: response.StatusCode}
	}
	return nil
}
func (a *HTTPServerClient) CreateFolder(ctx context.Context, id, root, name string) error {
	if !safeIdentifier(id) {
		return ErrInvalid
	}
	return a.request(ctx, "POST", prefix(id)+"/files/create-folder", map[string]any{"root": root, "name": name}, nil)
}
func (a *HTTPServerClient) RenameFile(ctx context.Context, id, root, from, to string) error {
	if !safeIdentifier(id) {
		return ErrInvalid
	}
	return a.request(ctx, "PUT", prefix(id)+"/files/rename", map[string]any{"root": root, "files": []map[string]string{{"from": from, "to": to}}}, nil)
}
func (a *HTTPServerClient) DeleteFile(ctx context.Context, id, root, name string) error {
	if !safeIdentifier(id) {
		return ErrInvalid
	}
	return a.request(ctx, "POST", prefix(id)+"/files/delete", map[string]any{"root": root, "files": []string{name}}, nil)
}
func (a *HTTPServerClient) signed(ctx context.Context, id, path string) (string, error) {
	if !safeIdentifier(id) {
		return "", ErrInvalid
	}
	var r struct {
		Attributes struct {
			URL string `json:"url"`
		} `json:"attributes"`
	}
	if err := a.request(ctx, "GET", prefix(id)+path, nil, &r); err != nil {
		return "", err
	}
	return r.Attributes.URL, nil
}
func (a *HTTPServerClient) transferURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.User != nil || u.Fragment != "" || (u.Scheme != "https" && !(a.allowTransferHTTP && u.Scheme == "http")) {
		return nil, ErrInvalid
	}
	origin := u.Scheme + "://" + u.Host
	for _, v := range a.transferOrigins {
		if v == origin {
			return u, nil
		}
	}
	return nil, ErrInvalid
}
func transferHTTP() *http.Client {
	return &http.Client{Timeout: 10 * time.Minute, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}
func (a *HTTPServerClient) download(ctx context.Context, raw string) (io.ReadCloser, error) {
	u, err := a.transferURL(raw)
	if err != nil {
		return nil, err
	}
	r, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return nil, ErrInvalid
	}
	response, err := transferHTTP().Do(r)
	if err != nil {
		return nil, fmt.Errorf("download failed")
	}
	if response.StatusCode != 200 {
		response.Body.Close()
		return nil, &APIError{Status: response.StatusCode}
	}
	return response.Body, nil
}
func (a *HTTPServerClient) DownloadFile(ctx context.Context, id, file string) (io.ReadCloser, error) {
	raw, err := a.signed(ctx, id, "/files/download?file="+url.QueryEscape(file))
	if err != nil {
		return nil, err
	}
	return a.download(ctx, raw)
}
func (a *HTTPServerClient) UploadFile(ctx context.Context, id, dir, name string, data []byte) error {
	if len(data) > MaxUploadBytes {
		return ErrInvalid
	}
	raw, err := a.signed(ctx, id, "/files/upload")
	if err != nil {
		return err
	}
	u, err := a.transferURL(raw)
	if err != nil || u.Path != "/upload/file" {
		return ErrInvalid
	}
	query := u.Query()
	query.Set("directory", dir)
	u.RawQuery = query.Encode()
	// Buffering is deliberately bounded by MaxUploadBytes; no local server files.
	var b bytes.Buffer
	form := multipart.NewWriter(&b)
	part, err := form.CreateFormFile("files", name)
	if err != nil {
		return ErrInvalid
	}
	if _, err = part.Write(data); err != nil {
		return err
	}
	if err = form.Close(); err != nil {
		return err
	}
	r, err := http.NewRequestWithContext(ctx, "POST", u.String(), &b)
	if err != nil {
		return ErrInvalid
	}
	r.Header.Set("Content-Type", form.FormDataContentType())
	response, err := transferHTTP().Do(r)
	if err != nil {
		return fmt.Errorf("upload failed")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return &APIError{Status: response.StatusCode}
	}
	return nil
}
func (a *HTTPServerClient) ListBackups(ctx context.Context, id string) ([]Backup, error) {
	if !safeIdentifier(id) {
		return nil, ErrInvalid
	}
	out := []Backup{}
	for page := 1; page <= 1000; page++ {
		var r struct {
			Data []struct {
				Attributes Backup `json:"attributes"`
			} `json:"data"`
			Meta struct {
				Pagination struct {
					TotalPages int `json:"total_pages"`
				} `json:"pagination"`
			} `json:"meta"`
		}
		if err := a.request(ctx, "GET", fmt.Sprintf("%s/backups?per_page=50&page=%d", prefix(id), page), nil, &r); err != nil {
			return nil, err
		}
		for _, v := range r.Data {
			out = append(out, v.Attributes)
		}
		if page >= r.Meta.Pagination.TotalPages {
			return out, nil
		}
	}
	return nil, ErrInvalid
}
func (a *HTTPServerClient) CreateBackup(ctx context.Context, id, name string) (Backup, error) {
	if !safeIdentifier(id) {
		return Backup{}, ErrInvalid
	}

	var capacity struct {
		Attributes struct {
			FeatureLimits struct {
				Backups int `json:"backups"`
			} `json:"feature_limits"`
		} `json:"attributes"`
	}
	if err := a.request(ctx, "GET", prefix(id), nil, &capacity); err != nil {
		return Backup{}, err
	}
	list, err := a.ListBackups(ctx, id)
	if err != nil {
		return Backup{}, err
	}
	if len(list) >= capacity.Attributes.FeatureLimits.Backups {
		return Backup{}, ErrConflict
	}
	var r struct {
		Attributes Backup `json:"attributes"`
	}
	err = a.request(ctx, "POST", prefix(id)+"/backups", map[string]any{"name": name, "is_locked": true}, &r)
	return r.Attributes, err
}
func (a *HTTPServerClient) RestoreBackup(ctx context.Context, id, backup string) error {
	if !safeIdentifier(id) {
		return ErrInvalid
	}
	if _, err := uuid.Parse(backup); err != nil {
		return ErrInvalid
	}
	return a.request(ctx, "POST", prefix(id)+"/backups/"+url.PathEscape(backup)+"/restore", map[string]bool{"truncate": true}, nil)
}
func (a *HTTPServerClient) DeleteBackup(ctx context.Context, id, backup string) error {
	// Our backups are locked to prevent Panel's rotating retention from evicting
	// another backup during creation. Explicit customer deletion unlocks first.
	if !safeIdentifier(id) {
		return ErrInvalid
	}
	if _, err := uuid.Parse(backup); err != nil {
		return ErrInvalid
	}
	list, err := a.ListBackups(ctx, id)
	if err != nil {
		return err
	}
	found := false
	for _, b := range list {
		if b.UUID == backup {
			found = true
			if b.Locked {
				if err := a.request(ctx, "POST", prefix(id)+"/backups/"+backup+"/lock", nil, nil); err != nil {
					return err
				}
			}
			break
		}
	}
	if !found {
		return ErrNotFound
	}
	return a.request(ctx, "DELETE", prefix(id)+"/backups/"+url.PathEscape(backup), nil, nil)
}
func (a *HTTPServerClient) DownloadBackup(ctx context.Context, id, backup string) (io.ReadCloser, error) {
	if _, err := uuid.Parse(backup); err != nil {
		return nil, ErrInvalid
	}
	raw, err := a.signed(ctx, id, "/backups/"+url.PathEscape(backup)+"/download")
	if err != nil {
		return nil, err
	}
	return a.download(ctx, raw)
}
