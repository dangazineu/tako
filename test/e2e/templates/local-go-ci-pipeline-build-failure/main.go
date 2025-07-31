//go:build ignore

package main

import (
	"fmt"
	"net/http"
	"non/existent/package" // This will cause a build failure
)

func handler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "Hello, Tako!")
}

func main() {
	http.HandleFunc("/", handler)
	http.ListenAndServe(":8080", nil)
}
