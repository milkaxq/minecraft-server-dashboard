package server

import "context"

type SoftwareOption struct {
	ServerType       string `json:"server_type"`
	MinecraftVersion string `json:"minecraft_version"`
	LoaderVersion    string `json:"loader_version,omitempty"`
}

func (s Service) SoftwareOptions(ctx context.Context) ([]SoftwareOption, error) {
	rows, err := s.DB.Query(ctx, `SELECT v.server_type,v.version,COALESCE((t.environment || v.environment)->>CASE v.server_type WHEN 'FABRIC' THEN 'LOADER_VERSION' WHEN 'FORGE' THEN 'FORGE_VERSION' WHEN 'NEOFORGE' THEN 'NEOFORGE_VERSION' END,'') FROM minecraft_versions v JOIN software_templates t ON t.server_type=v.server_type WHERE v.enabled AND t.enabled ORDER BY v.server_type,v.version`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []SoftwareOption{}
	for rows.Next() {
		var v SoftwareOption
		if err = rows.Scan(&v.ServerType, &v.MinecraftVersion, &v.LoaderVersion); err != nil {
			return nil, err
		}
		result = append(result, v)
	}
	return result, rows.Err()
}
