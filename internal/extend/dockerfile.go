package extend

type Dockerfile struct {
	Path string `toml:"path"`
	Args []Arg
}

type Arg struct {
	Name  string `toml:"name"`
	Value string `toml:"value"`
}
