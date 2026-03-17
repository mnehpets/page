package main

import (
	"context"
	"io/fs"
	"log"
	"net/http"
	"os"

	mnfs "github.com/mnehpets/fs"
	"github.com/mnehpets/http/endpoint"
	"github.com/mnehpets/page"
)

func main() {
	// Serve the local ./public directory as a content site.
	// Markdown and HTML files are rendered through the layout;
	// other files (CSS, images, etc.) are served as-is.
	root := os.DirFS("public")

	site, err := page.NewSite(root)
	if err != nil {
		log.Fatal("NewSite:", err)
	}

	noLayouts, err := mnfs.WithGlob("_layouts/*", mnfs.Disallowed)
	if err != nil {
		log.Fatal("WithGlob:", err)
	}
	public := mnfs.NewFilterFS(root, noLayouts)

	fsEndpoint := &endpoint.FileSystem{
		FS: func(_ context.Context, _ *http.Request) (fs.FS, error) {
			return public, nil
		},
	}
	endpoint.WithFileRenderer(site.FileRenderer())(fsEndpoint)
	endpoint.WithDirRenderer(site.DirRenderer())(fsEndpoint)

	mux := http.NewServeMux()
	mux.HandleFunc("/{path...}", endpoint.HandleFunc(fsEndpoint.Endpoint))

	log.Println("Content-site example listening on :8080")
	log.Println("Serving ./public — .md and .html files rendered via _layouts/")
	if _, err := fs.Stat(root, "."); err != nil {
		log.Println("warning: ./public does not exist or is not readable:", err)
	}

	if err := http.ListenAndServe(":8080", mux); err != nil {
		log.Fatal(err)
	}
}
