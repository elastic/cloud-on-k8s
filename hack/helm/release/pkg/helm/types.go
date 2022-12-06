package helm

type chart struct {
	Name         string       `json:"name"`
	Version      string       `json:"version"`
	Dependencies []dependency `json:"dependencies"`
}

type dependency struct {
	Name       string `json:"name"`
	Version    string `json:"version"`
	Repository string `json:"repository"`
}
