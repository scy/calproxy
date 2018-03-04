package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"time"
	"strconv"
	"crypto/sha512"
)

type Origin struct {
	URL        *url.URL
	RawContent string
	LastFetch  time.Time
	ticker *time.Ticker
}

func (o *Origin) Fetch() error {
	resp, err := http.Get(o.URL.String())
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	bytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	o.LastFetch = time.Now()
	o.RawContent = string(bytes)
	return nil
}

func (o *Origin) AutoUpdate(every time.Duration) {
	if o.ticker != nil {
		// Stop existing ticker.
		o.ticker.Stop()
	}
	o.ticker = time.NewTicker(every)
	go func() {
		for {
			<- o.ticker.C
			log.Print("updating from origin")
			err := o.Fetch()
			if err == nil {
				log.Print("updated successfully")
			} else {
				log.Print(err)
			}
		}
	}()
}

type Server struct {
	Origin *Origin
	Hash string
	secret string
}

func (s *Server) handler(w http.ResponseWriter, r *http.Request) {
	log.Printf("valid request from %s", r.RemoteAddr)
	fmt.Fprint(w, s.Origin.RawContent)
}

func (s *Server) ListenAndServe() error {
	s.secret = os.Getenv("CALPROXY_SECRET")
	s.Hash = fmt.Sprintf("%x", sha512.Sum512([]byte(s.secret + s.Origin.URL.String())))
	log.Printf("calendar will be served at %s.ics", s.Hash)
	http.HandleFunc(fmt.Sprintf("/%s.ics", s.Hash), s.handler)
	port, err := strconv.Atoi(os.Getenv("CALPROXY_PORT"))
	if err != nil {
		return err
	}
	listenAddr := fmt.Sprintf(":%d", port)
	log.Printf("starting to listen on %s", listenAddr)
	return http.ListenAndServe(listenAddr, nil)
}

func createOrigin() *Origin {
	originURL, err := url.Parse(os.Getenv("CALPROXY_ORIGIN"))
	if err != nil {
		log.Fatal(err)
	}
	return &Origin{
		URL: originURL,
	}
}

func main() {
	origin := createOrigin()
	log.Print("starting initial fetch")
	err := origin.Fetch()
	if err != nil {
		log.Fatal(err)
	}
	log.Print("initial fetch successful")
	s := Server{
		Origin: origin,
	}
	updSecs, err := strconv.Atoi(os.Getenv("CALPROXY_UPDATE_SECS"))
	if err != nil {
		updSecs = 60 * 16
		log.Printf("could not read CALPROXY_UPDATE_SECS, defaulting to update every %d seconds", updSecs)
	}
	origin.AutoUpdate(time.Duration(updSecs) * time.Second)
	log.Fatal(s.ListenAndServe())
}
