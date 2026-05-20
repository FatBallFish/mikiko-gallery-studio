package adminconfig

type Item struct {
	ConfigCategory string         `json:"config_category"`
	ConfigKey      string         `json:"config_key"`
	ConfigValue    map[string]any `json:"config_value"`
	Scope          string         `json:"scope"`
	Version        int64          `json:"version,omitempty"`
}

type Tab struct {
	TabKey   string `json:"tab_key"`
	TabName  string `json:"tab_name"`
	Version  int64  `json:"version"`
	Items    []Item `json:"items"`
	ReadOnly bool   `json:"read_only,omitempty"`
}

type UpdateTabRequest struct {
	TabKey    string
	Version   int64
	Items     []Item
	UpdatedBy int64
}
