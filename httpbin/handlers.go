package httpbin

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

var acceptedMediaTypes = []string{
	"image/webp",
	"image/svg+xml",
	"image/jpeg",
	"image/png",
	"image/",
}

// Index renders an HTML index page
func (h *HTTPBin) Index(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}
	writeHTML(w, MustAsset("index.html"), http.StatusOK)
}

// FormsPost renders an HTML form that submits a request to the /post endpoint
func (h *HTTPBin) FormsPost(w http.ResponseWriter, r *http.Request) {
	writeHTML(w, MustAsset("forms-post.html"), http.StatusOK)
}

// UTF8 renders an HTML encoding stress test
func (h *HTTPBin) UTF8(w http.ResponseWriter, r *http.Request) {
	writeHTML(w, MustAsset("utf8.html"), http.StatusOK)
}

// Get handles HTTP GET requests
func (h *HTTPBin) Get(w http.ResponseWriter, r *http.Request) {
	resp := &getResponse{
		Args:    r.URL.Query(),
		Headers: r.Header,
		Origin:  getOrigin(r),
		URL:     getURL(r).String(),
	}
	body, _ := json.Marshal(resp)
	writeJSON(w, body, http.StatusOK)
}

// RequestWithBody handles POST, PUT, and PATCH requests
func (h *HTTPBin) RequestWithBody(w http.ResponseWriter, r *http.Request) {
	resp := &bodyResponse{
		Args:    r.URL.Query(),
		Headers: r.Header,
		Origin:  getOrigin(r),
		URL:     getURL(r).String(),
	}

	err := parseBody(w, r, resp, h.options.MaxMemory)
	if err != nil {
		http.Error(w, fmt.Sprintf("error parsing request body: %s", err), http.StatusBadRequest)
		return
	}

	body, _ := json.Marshal(resp)
	writeJSON(w, body, http.StatusOK)
}

// Gzip returns a gzipped response
func (h *HTTPBin) Gzip(w http.ResponseWriter, r *http.Request) {
	resp := &gzipResponse{
		Headers: r.Header,
		Origin:  getOrigin(r),
		Gzipped: true,
	}
	body, _ := json.Marshal(resp)

	buf := &bytes.Buffer{}
	gzw := gzip.NewWriter(buf)
	gzw.Write(body)
	gzw.Close()

	gzBody := buf.Bytes()

	w.Header().Set("Content-Encoding", "gzip")
	writeJSON(w, gzBody, http.StatusOK)
}

// Deflate returns a gzipped response
func (h *HTTPBin) Deflate(w http.ResponseWriter, r *http.Request) {
	resp := &deflateResponse{
		Headers:  r.Header,
		Origin:   getOrigin(r),
		Deflated: true,
	}
	body, _ := json.Marshal(resp)

	buf := &bytes.Buffer{}
	w2, _ := flate.NewWriter(buf, flate.DefaultCompression)
	w2.Write(body)
	w2.Close()

	compressedBody := buf.Bytes()

	w.Header().Set("Content-Encoding", "deflate")
	writeJSON(w, compressedBody, http.StatusOK)
}

// IP echoes the IP address of the incoming request
func (h *HTTPBin) IP(w http.ResponseWriter, r *http.Request) {
	body, _ := json.Marshal(&ipResponse{
		Origin: getOrigin(r),
	})
	writeJSON(w, body, http.StatusOK)
}

// UserAgent echoes the incoming User-Agent header
func (h *HTTPBin) UserAgent(w http.ResponseWriter, r *http.Request) {
	body, _ := json.Marshal(&userAgentResponse{
		UserAgent: r.Header.Get("User-Agent"),
	})
	writeJSON(w, body, http.StatusOK)
}

// Headers echoes the incoming request headers
func (h *HTTPBin) Headers(w http.ResponseWriter, r *http.Request) {
	body, _ := json.Marshal(&headersResponse{
		Headers: r.Header,
	})
	writeJSON(w, body, http.StatusOK)
}

// Status responds with the specified status code. TODO: support random choice
// from multiple, optionally weighted status codes.
func (h *HTTPBin) Status(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) != 3 {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}
	code, err := strconv.Atoi(parts[2])
	if err != nil {
		http.Error(w, "Invalid status", http.StatusBadRequest)
		return
	}

	type statusCase struct {
		headers map[string]string
		body    []byte
	}

	redirectHeaders := &statusCase{
		headers: map[string]string{
			"Location": "/redirect/1",
		},
	}
	notAcceptableBody, _ := json.Marshal(map[string]interface{}{
		"message": "Client did not request a supported media type",
		"accept":  acceptedMediaTypes,
	})

	specialCases := map[int]*statusCase{
		301: redirectHeaders,
		302: redirectHeaders,
		303: redirectHeaders,
		305: redirectHeaders,
		307: redirectHeaders,
		401: &statusCase{
			headers: map[string]string{
				"WWW-Authenticate": `Basic realm="Fake Realm"`,
			},
		},
		402: &statusCase{
			body: []byte("Fuck you, pay me!"),
			headers: map[string]string{
				"X-More-Info": "http://vimeo.com/22053820",
			},
		},
		406: &statusCase{
			body: notAcceptableBody,
			headers: map[string]string{
				"Content-Type": jsonContentType,
			},
		},
		407: &statusCase{
			headers: map[string]string{
				"Proxy-Authenticate": `Basic realm="Fake Realm"`,
			},
		},
		418: &statusCase{
			body: []byte("I'm a teapot!"),
			headers: map[string]string{
				"X-More-Info": "http://tools.ietf.org/html/rfc2324",
			},
		},
	}

	if specialCase, ok := specialCases[code]; ok {
		if specialCase.headers != nil {
			for key, val := range specialCase.headers {
				w.Header().Set(key, val)
			}
		}
		w.WriteHeader(code)
		if specialCase.body != nil {
			w.Write(specialCase.body)
		}
	} else {
		w.WriteHeader(code)
	}
}

// ResponseHeaders responds with a map of header values
func (h *HTTPBin) ResponseHeaders(w http.ResponseWriter, r *http.Request) {
	args := r.URL.Query()
	for k, vs := range args {
		for _, v := range vs {
			w.Header().Add(http.CanonicalHeaderKey(k), v)
		}
	}
	body, _ := json.Marshal(args)
	if contentType := w.Header().Get("Content-Type"); contentType == "" {
		w.Header().Set("Content-Type", jsonContentType)
	}
	w.Write(body)
}

func redirectLocation(r *http.Request, relative bool, n int) string {
	var location string
	var path string

	if n < 1 {
		path = "/get"
	} else if relative {
		path = fmt.Sprintf("/relative-redirect/%d", n)
	} else {
		path = fmt.Sprintf("/absolute-redirect/%d", n)
	}

	if relative {
		location = path
	} else {
		u := getURL(r)
		u.Path = path
		u.RawQuery = ""
		location = u.String()
	}

	return location
}

func doRedirect(w http.ResponseWriter, r *http.Request, relative bool) {
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) != 3 {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}
	n, err := strconv.Atoi(parts[2])
	if err != nil || n < 1 {
		http.Error(w, "Invalid redirect", http.StatusBadRequest)
		return
	}

	w.Header().Set("Location", redirectLocation(r, relative, n-1))
	w.WriteHeader(http.StatusFound)
}

// Redirect responds with 302 redirect a given number of times. Defaults to a
// relative redirect, but an ?absolute=true query param will trigger an
// absolute redirect.
func (h *HTTPBin) Redirect(w http.ResponseWriter, r *http.Request) {
	params := r.URL.Query()
	relative := strings.ToLower(params.Get("absolute")) != "true"
	doRedirect(w, r, relative)
}

// RelativeRedirect responds with an HTTP 302 redirect a given number of times
func (h *HTTPBin) RelativeRedirect(w http.ResponseWriter, r *http.Request) {
	doRedirect(w, r, true)
}

// AbsoluteRedirect responds with an HTTP 302 redirect a given number of times
func (h *HTTPBin) AbsoluteRedirect(w http.ResponseWriter, r *http.Request) {
	doRedirect(w, r, false)
}

// Cookies responds with the cookies in the incoming request
func (h *HTTPBin) Cookies(w http.ResponseWriter, r *http.Request) {
	resp := cookiesResponse{}
	for _, c := range r.Cookies() {
		resp[c.Name] = c.Value
	}
	body, _ := json.Marshal(resp)
	writeJSON(w, body, http.StatusOK)
}

// SetCookies sets cookies as specified in query params and redirects to
// Cookies endpoint
func (h *HTTPBin) SetCookies(w http.ResponseWriter, r *http.Request) {
	params := r.URL.Query()
	for k := range params {
		http.SetCookie(w, &http.Cookie{
			Name:     k,
			Value:    params.Get(k),
			HttpOnly: true,
		})
	}
	w.Header().Set("Location", "/cookies")
	w.WriteHeader(http.StatusFound)
}

// DeleteCookies deletes cookies specified in query params and redirects to
// Cookies endpoint
func (h *HTTPBin) DeleteCookies(w http.ResponseWriter, r *http.Request) {
	params := r.URL.Query()
	for k := range params {
		http.SetCookie(w, &http.Cookie{
			Name:     k,
			Value:    params.Get(k),
			HttpOnly: true,
			MaxAge:   -1,
			Expires:  time.Now().Add(-1 * 24 * 365 * time.Hour),
		})
	}
	w.Header().Set("Location", "/cookies")
	w.WriteHeader(http.StatusFound)
}

// BasicAuth requires basic authentication
func (h *HTTPBin) BasicAuth(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) != 4 {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}
	expectedUser := parts[2]
	expectedPass := parts[3]

	givenUser, givenPass, _ := r.BasicAuth()

	status := http.StatusOK
	authorized := givenUser == expectedUser && givenPass == expectedPass
	if !authorized {
		status = http.StatusUnauthorized
		w.Header().Set("WWW-Authenticate", `Basic realm="Fake Realm"`)
	}

	body, _ := json.Marshal(&authResponse{
		Authorized: authorized,
		User:       givenUser,
	})
	writeJSON(w, body, status)
}

// HiddenBasicAuth requires HTTP Basic authentication but returns a status of
// 404 if the request is unauthorized
func (h *HTTPBin) HiddenBasicAuth(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) != 4 {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}
	expectedUser := parts[2]
	expectedPass := parts[3]

	givenUser, givenPass, _ := r.BasicAuth()

	authorized := givenUser == expectedUser && givenPass == expectedPass
	if !authorized {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	body, _ := json.Marshal(&authResponse{
		Authorized: authorized,
		User:       givenUser,
	})
	writeJSON(w, body, http.StatusOK)
}

// DigestAuth is not yet implemented, and returns 501 Not Implemented. It
// appears that stdlib support for working with digest authentication is
// lacking, and I'm not yet ready to implement it myself.
func (h *HTTPBin) DigestAuth(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) != 5 {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}
	http.Error(w, "Not Implemented", http.StatusNotImplemented)
}

// Stream responds with max(n, 100) lines of JSON-encoded request data.
func (h *HTTPBin) Stream(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) != 3 {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}
	n, err := strconv.Atoi(parts[2])
	if err != nil {
		http.Error(w, "Invalid integer", http.StatusBadRequest)
		return
	}

	if n > 100 {
		n = 100
	} else if n < 1 {
		n = 1
	}

	resp := &streamResponse{
		Args:    r.URL.Query(),
		Headers: r.Header,
		Origin:  getOrigin(r),
		URL:     getURL(r).String(),
	}

	f := w.(http.Flusher)
	for i := 0; i < n; i++ {
		resp.ID = i
		line, _ := json.Marshal(resp)
		w.Write(line)
		w.Write([]byte("\n"))
		f.Flush()
	}
}

// Delay waits for a given amount of time before responding, where the time may
// be specified as a golang-style duration or seconds in floating point.
func (h *HTTPBin) Delay(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) != 3 {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	delay, err := parseBoundedDuration(parts[2], 0, h.options.MaxResponseTime)
	if err != nil {
		http.Error(w, "Invalid duration", http.StatusBadRequest)
		return
	}

	<-time.After(delay)
	h.RequestWithBody(w, r)
}

// Drip returns data over a duration after an optional initial delay, then
// (optionally) returns with the given status code.
func (h *HTTPBin) Drip(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	duration := time.Duration(0)
	delay := time.Duration(0)
	numbytes := int64(10)
	code := http.StatusOK

	var err error

	userDuration := q.Get("duration")
	if userDuration != "" {
		duration, err = parseBoundedDuration(userDuration, 0, h.options.MaxResponseTime)
		if err != nil {
			http.Error(w, "Invalid duration", http.StatusBadRequest)
			return
		}
	}

	userDelay := q.Get("delay")
	if userDelay != "" {
		delay, err = parseBoundedDuration(userDelay, 0, h.options.MaxResponseTime)
		if err != nil {
			http.Error(w, "Invalid delay", http.StatusBadRequest)
			return
		}
	}

	userNumBytes := q.Get("numbytes")
	if userNumBytes != "" {
		numbytes, err = strconv.ParseInt(userNumBytes, 10, 64)
		if err != nil || numbytes <= 0 || numbytes > h.options.MaxResponseSize {
			http.Error(w, "Invalid numbytes", http.StatusBadRequest)
			return
		}
	}

	userCode := q.Get("code")
	if userCode != "" {
		code, err = strconv.Atoi(userCode)
		if err != nil || code < 100 || code >= 600 {
			http.Error(w, "Invalid code", http.StatusBadRequest)
			return
		}
	}

	if duration+delay > h.options.MaxResponseTime {
		http.Error(w, "Too much time", http.StatusBadRequest)
		return
	}

	pause := duration / time.Duration(numbytes)

	<-time.After(delay)

	w.WriteHeader(code)
	w.Header().Set("Content-Type", "application/octet-stream")

	f := w.(http.Flusher)
	for i := int64(0); i < numbytes; i++ {
		w.Write([]byte("*"))
		f.Flush()
		<-time.After(pause)
	}
}

// Range returns up to N bytes, with support for HTTP Range requests.
//
// This departs from httpbin by not supporting the chunk_size or duration
// parameters.
func (h *HTTPBin) Range(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) != 3 {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	numBytes, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Add("ETag", fmt.Sprintf("range%d", numBytes))
	w.Header().Add("Accept-Ranges", "bytes")

	if numBytes <= 0 || numBytes > h.options.MaxMemory {
		http.Error(w, "Invalid number of bytes", http.StatusBadRequest)
		return
	}

	content := &syntheticReadSeeker{
		numBytes:    numBytes,
		byteFactory: func(offset int64) byte { return byte(97 + (offset % 26)) },
	}
	var modtime time.Time
	http.ServeContent(w, r, "", modtime, content)
}

// HTML renders a basic HTML page
func (h *HTTPBin) HTML(w http.ResponseWriter, r *http.Request) {
	writeHTML(w, MustAsset("moby.html"), http.StatusOK)
}

// Robots renders a basic robots.txt file
func (h *HTTPBin) Robots(w http.ResponseWriter, r *http.Request) {
	robotsTxt := []byte(`User-agent: *
Disallow: /deny
`)
	writeResponse(w, http.StatusOK, "text/plain", robotsTxt)
}

// Deny renders a basic page that robots should never access
func (h *HTTPBin) Deny(w http.ResponseWriter, r *http.Request) {
	writeResponse(w, http.StatusOK, "text/plain", []byte(`YOU SHOULDN'T BE HERE`))
}

// Cache returns a 304 if an If-Modified-Since or an If-None-Match header is
// present, otherwise returns the same response as Get.
func (h *HTTPBin) Cache(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("If-Modified-Since") != "" || r.Header.Get("If-None-Match") != "" {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	// Did we get an additional /cache/N path parameter? If so, validate it
	// and set the Cache-Control header.
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) == 3 {
		seconds, err := strconv.ParseInt(parts[2], 10, 64)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Add("Cache-Control", fmt.Sprintf("public, max-age=%d", seconds))
	}

	lastModified := time.Now().Format(time.RFC1123)
	w.Header().Add("Last-Modified", lastModified)
	w.Header().Add("ETag", sha1hash(lastModified))
	h.Get(w, r)
}

// CacheControl sets a Cache-Control header for N seconds for /cache/N requests
func (h *HTTPBin) CacheControl(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) != 3 {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	seconds, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Add("Cache-Control", fmt.Sprintf("public, max-age=%d", seconds))
	h.Get(w, r)
}

// ETag assumes the resource has the given etag and response to If-None-Match
// and If-Match headers appropriately.
func (h *HTTPBin) ETag(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) != 3 {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	etag := parts[2]
	w.Header().Set("ETag", fmt.Sprintf(`"%s"`, etag))

	// TODO: This mostly duplicates the work of Get() above, should this be
	// pulled into a little helper?
	resp := &getResponse{
		Args:    r.URL.Query(),
		Headers: r.Header,
		Origin:  getOrigin(r),
		URL:     getURL(r).String(),
	}
	body, _ := json.Marshal(resp)

	// Let http.ServeContent deal with If-None-Match and If-Match headers:
	// https://golang.org/pkg/net/http/#ServeContent
	http.ServeContent(w, r, "response.json", time.Now(), bytes.NewReader(body))
}
