package main

import "net/http"

type staticPage string

func (p staticPage) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(p))
}
