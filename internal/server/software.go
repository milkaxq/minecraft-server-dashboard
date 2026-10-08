package server

import "context"

type SoftwareOption struct {
	ServerType       string `json:"server_type"`
	MinecraftVersion string `json:"minecraft_version"`
}

func (s Service) SoftwareOptions(ctx context.Context) ([]SoftwareOption, error) {
	rows, err := s.DB.Query(ctx, `SELECT v.server_type,v.version FROM minecraft_versions v JOIN software_templates t ON t.server_type=v.server_type WHERE v.enabled AND t.enabled ORDER BY v.server_type,v.version`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []SoftwareOption{}
	for rows.Next() {
		var v SoftwareOption
		if err = rows.Scan(&v.ServerType, &v.MinecraftVersion); err != nil {
			return nil, err
		}
		result = append(result, v)
	}
	return result, rows.Err()
}
