package main

import (
	"fmt"
	"os"

	nucleusruntime "nucleus-backend/internal/runtime"
)

func main() {
	info := nucleusruntime.DefaultBuildInfo()
	_, _ = fmt.Fprintf(os.Stdout, "%s scaffold %s\n", info.Service, info.Version)
}
