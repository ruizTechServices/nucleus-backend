package filesystem

const (
	ToolList = "filesystem.list"
	ToolRead = "filesystem.read"
)

type ListRequest struct {
	Path string `json:"path"`
}

type Entry struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	IsDir bool   `json:"is_dir"`
}

type ListResponse struct {
	Path    string  `json:"path"`
	Entries []Entry `json:"entries"`
}

type ReadRequest struct {
	Path string `json:"path"`
}

type ReadResponse struct {
	Path     string `json:"path"`
	Content  string `json:"content"`
	Encoding string `json:"encoding"`
}
