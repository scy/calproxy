package main

import (
	"crypto/sha512"
	"encoding/base64"
	"fmt"
	"github.com/luxifer/ical"
	"golang.org/x/crypto/ssh/terminal"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

func escapeCalParamValue(val string) string {
	// Property param values may not include double quotes.
	val = strings.Replace(val, "\"", "", -1)
	return "\"" + val + "\""
}

func paramToString(param *ical.Param) string {
	for idx, val := range param.Values {
		param.Values[idx] = escapeCalParamValue(val)
	}
	return strings.Join(param.Values, ",")
}

func paramsToString(params map[string]*ical.Param) string {
	stringSlice := make([]string, 0)
	for name, param := range params {
		stringSlice = append(stringSlice, fmt.Sprintf("%s=%s", name, paramToString(param)))
	}
	return strings.Join(stringSlice, ";")
}

func propToString(prop *ical.Property) string {
	paramStr := paramsToString(prop.Params)
	if paramStr != "" {
		paramStr = ";" + paramStr
	}
	return fmt.Sprintf("%s%s:%s", prop.Name, paramStr, prop.Value)
}

func filteredHeaders(headers []*ical.Property) string {
	lines := make([]string, 0)
	allow := false
	for _, prop := range headers {
		if prop.Name == "BEGIN" && prop.Value == "VTIMEZONE" {
			allow = true
		}
		if allow {
			lines = append(lines, propToString(prop))
		}
		if prop.Name == "END" && prop.Value == "VTIMEZONE" {
			allow = false
		}
	}
	return strings.Join(lines, "\n")
}

func censoredEvent(event *ical.Event, calID string, fbTitle string) string {
	lines := make([]string, 0)
	lines = append(lines, "BEGIN:VEVENT")
	lines = append(lines, propToString(&ical.Property{
		Name:  "SUMMARY",
		Value: fbTitle,
	}))
	for _, prop := range event.Properties {
		switch prop.Name {
		case "DTSTART", "DTEND", "DURATION", "RRULE":
			lines = append(lines, propToString(prop))
		case "UID":
			prop.Value = fmt.Sprintf("calproxy-%s-%s", prop.Value, calID)
			lines = append(lines, propToString(prop))
		}
	}
	lines = append(lines, "END:VEVENT")
	return strings.Join(lines, "\n")
}

type Origin struct {
	url        *url.URL
	id         string
	auth       string
	RawContent string
	LastFetch  time.Time
	FreeBusy   string
	ticker     *time.Ticker
}

func (o *Origin) updateFreeBusy() error {
	cal, err := ical.Parse(strings.NewReader(o.RawContent))
	if err != nil {
		return err
	}
	fbTitle := os.Getenv("CALPROXY_FB_TITLE")
	lines := make([]string, 0)
	lines = append(lines, "BEGIN:VCALENDAR")
	lines = append(lines, filteredHeaders(cal.Properties))
	for _, event := range cal.Events {
		lines = append(lines, censoredEvent(event, o.id, fbTitle))
	}
	lines = append(lines, "END:VCALENDAR")
	o.FreeBusy = strings.Join(lines, "\n")
	return nil
}

func (o *Origin) GetID() string {
	return o.id
}

func (o *Origin) SetURL(theURL *url.URL) {
	if theURL.User != nil {
		// Prepare a basic auth header value to prevent leaking the password in error messages, see
		// <https://github.com/golang/go/issues/24572>
		auth := theURL.User.Username()
		if pass, passSet := theURL.User.Password(); passSet {
			auth += ":" + pass
		}
		o.auth = "Basic " + base64.StdEncoding.EncodeToString([]byte(auth))
		cleanURL := &url.URL{}
		*cleanURL = *theURL
		cleanURL.User = nil
		o.url = cleanURL
	} else {
		o.auth = ""
		o.url = theURL
	}
	o.id = fmt.Sprintf("%x", sha512.Sum512([]byte(theURL.String())))
}

func (o *Origin) Fetch() error {
	client := &http.Client{}
	req, err := http.NewRequest("GET", o.url.String(), nil)
	if err != nil {
		return err
	}
	if o.auth != "" {
		req.Header.Set("Authorization", o.auth)
	}
	resp, err := client.Do(req)
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

func (o *Origin) FetchAndParse() error {
	if err := o.Fetch(); err != nil {
		return err
	}
	if err := o.updateFreeBusy(); err != nil {
		return err
	}
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
			<-o.ticker.C
			log.Print("updating from origin")
			err := o.FetchAndParse()
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
}

func (s *Server) calHandler(w http.ResponseWriter, r *http.Request) {
	log.Printf("valid calendar request from %s", r.RemoteAddr)
	fmt.Fprint(w, s.Origin.RawContent)
}

func (s *Server) freeBusyHandler(w http.ResponseWriter, r *http.Request) {
	log.Printf("valid free/busy request from %s", r.RemoteAddr)
	fmt.Fprint(w, s.Origin.FreeBusy)
}

func (s *Server) ListenAndServe() error {
	secret := os.Getenv("CALPROXY_SECRET")
	hash := fmt.Sprintf("%x", sha512.Sum512([]byte(secret+s.Origin.GetID())))
	log.Printf("calendar will be served at %s.ics", hash)
	http.HandleFunc(fmt.Sprintf("/%s.ics", hash), s.calHandler)
	log.Printf("free/busy will be served at %s.ics", s.Origin.GetID())
	http.HandleFunc(fmt.Sprintf("/%s.ics", s.Origin.GetID()), s.freeBusyHandler)
	port, err := strconv.Atoi(os.Getenv("CALPROXY_PORT"))
	if err != nil {
		return err
	}
	listenAddr := fmt.Sprintf(":%d", port)
	log.Printf("starting to listen on %s", listenAddr)
	return http.ListenAndServe(listenAddr, nil)
}

func createOrigin() *Origin {
	originStr := os.Getenv("CALPROXY_ORIGIN")
	if originStr == "" {
		fmt.Print("CALPROXY_ORIGIN not set, enter origin URL (will not be shown): ")
		bytes, err := terminal.ReadPassword(int(os.Stdin.Fd()))
		if err != nil {
			log.Fatal(err)
		}
		originStr = string(bytes)
		fmt.Println()
	}
	originURL, err := url.Parse(originStr)
	if err != nil {
		log.Fatal(err)
	}
	origin := Origin{}
	origin.SetURL(originURL)
	return &origin
}

func main() {
	origin := createOrigin()
	log.Print("starting initial fetch")
	err := origin.FetchAndParse()
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
