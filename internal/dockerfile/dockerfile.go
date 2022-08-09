package dockerfile

type Dockerfile struct {
	Path string `toml:"path"`
	Args []Arg
	// TODO: add args
}

type Arg struct {
	Name  string
	Value string
}
