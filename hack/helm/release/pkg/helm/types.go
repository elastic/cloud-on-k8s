package helm

type charts []chart

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

func (cs charts) chartNames() []string {
	names := make([]string, len(cs))
	for i, chart := range cs {
		names[i] = chart.Name
	}
	return names
}
