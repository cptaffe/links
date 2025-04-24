package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

var linksPath = flag.String("links", "", "path to links file")
var address = flag.String("address", ":8080", "address to listen on, :8080 by default")

type Link struct {
	Short string `json:"short"`
	Long  string `json:"long"`
}

type Links struct {
	Links  []Link
	lookup map[string]*Link
}

func (l *Links) Get(short string) (*Link, bool) {
	if l.lookup == nil {
		l.buildLookup()
	}
	link, ok := l.lookup[short]
	return link, ok
}

func (l *Links) buildLookup() {
	l.lookup = make(map[string]*Link)
	for i := range l.Links {
		link := &l.Links[i]
		l.lookup[link.Short] = link
	}
}

type Server struct {
	links *Links
}

func NewServer(links *Links) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /l/{short}", &Server{links: links})
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Add("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprint(w, "ok")
	})
	return mux
}

func (l *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	headers := w.Header()
	headers.Add("Content-Type", "text/html; charset=utf-8")
	headers.Add("Cache-Control", "private; max-age=90")
	headers.Add("Referrer-Policy", "unsafe-url")

	link, ok := l.links.Get(r.PathValue("short"))
	if !ok {
		http.NotFound(w, r)
		return
	}

	http.Redirect(w, r, link.Long, http.StatusMovedPermanently)
}

func main() {
	flag.Parse()

	f, err := os.Open(*linksPath)
	if err != nil {
		log.Fatalf("open links file %s: %v", *linksPath, err)
	}
	var links Links
	err = json.NewDecoder(f).Decode(&links)
	if err != nil {
		log.Fatalf("decode links file %s: %v", *linksPath, err)
	}

	server := http.Server{Addr: *address, Handler: NewServer(&links)}

	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	log.Println("starting server")
	go func() {
		err = server.ListenAndServe()
		switch err {
		case nil, http.ErrServerClosed:
		default:
			log.Fatal("listen and serve", err)
		}
	}()

	<-done
	log.Println("stopping server")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = server.Shutdown(ctx)
	if err != nil {
		log.Fatalf("shutdown server: %v", err)
	}
	log.Println("server stopped")
}
