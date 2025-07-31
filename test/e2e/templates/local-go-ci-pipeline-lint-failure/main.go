//go:build ignore

package main

import (
	"fmt"
	"net/http"
)

func handler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "Hello, Tako!")
}

func main() {
	// Intentional vet error: unreachable code
	return
	http.HandleFunc("/", handler)
	http.ListenAndServe(":8080", nil)
}
