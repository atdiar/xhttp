package upload

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"

	"github.com/atdiar/bottleneck"
	"github.com/atdiar/errors"
	"github.com/atdiar/xhttp"
	"github.com/atdiar/xhttp/handlers/session"
)

var (
	FileNameHeader    = http.CanonicalHeaderKey("filename")
	FileSizeHeader    = http.CanonicalHeaderKey("filesize")
	UploadIDHeader    = http.CanonicalHeaderKey("uploadid")
	ChunkOffsetHeader = http.CanonicalHeaderKey("chunkoffset")
	ChunksTotalHeader = http.CanonicalHeaderKey("chunkstotal")
	ChunkSizeHeader   = http.CanonicalHeaderKey("chunksize")

	ErrMissingUploadID    = errors.New("uploadid header missing")
	ErrMissingFilename    = errors.New("filename header missing")
	ErrMissingFilesize    = errors.New("filesize header missing")
	ErrMissingChunkOffset = errors.New("chunkoffset header missing")
	ErrMissingChunksTotal = errors.New("chunkstotal header missing")
	ErrMissingChunksize   = errors.New("chunksize header missing")

	TicketKey = "uploadticket"
)

// ParseUpload parses a submitted form-data POST or PUT request, uploading any submitted
// file within the limits defined for the endpoint in terms of upload size.
func (h ChunkHandler) ParseUpload( w http.ResponseWriter, r *http.Request) (ParseResult, error) {
	onerror := newCanceler()
	f := h.Handler.Form
	// Let's get the uploader id

	if !h.Handler.Session.Loaded(r.Context()) {
		return ParseResult{nil, onerror}, ErrParsingFailed.Wraps(errors.New("uploader session is not loaded"))
	}
	uploaderid, err := h.Handler.Session.ID()
	if err != nil {
		return ParseResult{nil, onerror}, ErrParsingFailed.Wraps(errors.New("No session ID found. Unable to retrieve uploader session id").Wraps(err))
	}

	//  Let's try toretrieve the headers that describe the upload and then load the
	// corresponding upload session if it exists.
	// The following are values for the chunk upload that are initially nil and
	// should only be modified once as only one file should be uploaded at a time.
	var (
		uploadFileCreated bool

		uploadid    string
		filename    string
		filesize    string
		chunksize   string
		chunkoffset string
		chunkstotal string
	)
	ruploadid, ok := r.Header[UploadIDHeader]
	if !ok {
		return ParseResult{nil, onerror}, ErrParsingFailed.Wraps(ErrMissingUploadID)
	}
	uploadid = ruploadid[0]

	rfilename, ok := r.Header[FileNameHeader]
	if !ok {
		return ParseResult{nil, onerror}, ErrParsingFailed.Wraps(ErrMissingFilename)
	}
	filename = rfilename[0]

	rfilesize, ok := r.Header[FileSizeHeader]
	if !ok {
		return ParseResult{nil, onerror}, ErrParsingFailed.Wraps(ErrMissingFilesize)
	}
	filesize = rfilesize[0]

	rchunksize, ok := r.Header[ChunkSizeHeader]
	if !ok {
		return ParseResult{nil, onerror}, ErrParsingFailed.Wraps(ErrMissingChunksize)
	}
	chunksize = rchunksize[0]

	rchunkoffset, ok := r.Header[ChunkOffsetHeader]
	if !ok {
		return ParseResult{nil, onerror}, ErrParsingFailed.Wraps(ErrMissingChunkOffset)
	}
	chunkoffset = rchunkoffset[0]

	rchunkstotal, ok := r.Header[ChunksTotalHeader]
	if !ok {
		return ParseResult{nil, onerror}, ErrParsingFailed.Wraps(ErrMissingChunksTotal)
	}
	chunkstotal = rchunkstotal[0]


// Let's try to load the upload session
	err = session.LoadServerOnly(r, uploadid, &h.Session)
	if err != nil {
		return ParseResult{nil, onerror}, ErrParsingFailed.Wraps(err)
	}

	contentType, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || !strings.HasPrefix(contentType, "multipart/") {
		return ParseResult{f, onerror}, errors.New("Content-Type error : expecting a multipart message")
	}
	if _, ok := params["boundary"]; !ok {
		return ParseResult{f, onerror}, ErrNoBoundary
	}

	reader := multipart.NewReader(r.Body, params["boundary"])
	if len(f) == 0 {
		return ParseResult{nil, onerror}, ErrServerFormInvalid
	}
	for fieldIndex := 0; fieldIndex < len(f); fieldIndex++ {
		p, err := reader.NextPart()
		if err != nil {
			if err != io.EOF {
				return ParseResult{nil, onerror}, ErrParsingFailed.Wraps(err)
			}
			for j := fieldIndex; j < len(f); j++ {
				if !f[fieldIndex].Required {
					continue
				} else {
					return ParseResult{nil, onerror}, ErrClientFormInvalid.Wraps(errors.New("upload form sent is missing a required field: " + f[fieldIndex].Name))
				}
			}
			return ParseResult{f, onerror}, nil
		}

		contentDisposition, _, _ := mime.ParseMediaType(p.Header.Get("Content-Disposition"))
		if contentDisposition != "form-data" {
			return ParseResult{nil, onerror}, ErrClientFormInvalid.Wraps(errors.New("Submitted form has bad formatting. Expecting Content-Disposition Header to be form-data for each part."))
		}

		name := p.FormName()

		for i := fieldIndex; i < len(f); i++ {
			if name != f[fieldIndex].Name {
				if !f[fieldIndex].Required {
					fieldIndex++
					continue
				} else {
					return ParseResult{f, onerror}, ErrClientFormInvalid.Wraps(errors.New("Client Error : upload form submitted  is missing a required field " + f[fieldIndex].Name + " or fields are sent out-of-order"))
				}
			}
			fieldIndex = i

			// Let's check the data content type
			contentType, _, err := mime.ParseMediaType(p.Header.Get("Content-Type"))
			if err != nil {
				buf := bufio.NewReader(p)
				peeksize := 512
				if f[fieldIndex].SizeLimit < int64(peeksize) {
					peeksize = int(f[fieldIndex].SizeLimit)
				}
				sniff, _ := buf.Peek(peeksize)
				contentType = http.DetectContentType(sniff)
				if !f[fieldIndex].AllowedContentTypes.Contains(contentType, false) {
					return ParseResult{f, onerror}, errors.New("Unsupported Content-Type")
				}
				f[fieldIndex].ContentType = contentType
			}
			if !f[fieldIndex].AllowedContentTypes.Contains(contentType, false) {
				return ParseResult{nil, onerror}, ErrClientFormInvalid.Wraps(ErrBadContentType)
			}
			f[fieldIndex].ContentType = contentType

			// Let's retrieve the data and make sure it fits within the size limit
			// If the data is of content-type multipart/mixed, it means it is a
			// multipart message comprised of different files.
			if contentType == "multipart/mixed" {
				return ParseResult{nil, onerror}, ErrServerFormInvalid.Wraps(errors.New("Chunked upload does not support multiple file upload"))
			}

			pr := io.LimitReader(p, f[fieldIndex].SizeLimit)
			if f[fieldIndex].Files != nil {
				if uploadFileCreated {
					return ParseResult{nil, onerror}, ErrServerFormInvalid.Wraps(errors.New("Form is malformed server side. Only one file upload field is allowed for chunk uploads"))
				}

				obj := NewFile(pr, string(filename), contentType, uploaderid, f[fieldIndex].Path)

				obj.UploadID = uploadid

				choff, err := strconv.ParseInt(chunkoffset, 10, 64)
				if err != nil {
					return ParseResult{nil, onerror}, ErrParsingFailed.Wraps(err)
				}
				obj.ChunkOffset = choff

				chtot, err := strconv.ParseInt(chunkstotal, 10, 64)
				if err != nil {
					return ParseResult{nil, onerror}, ErrParsingFailed.Wraps(err)
				}
				obj.ChunksTotal = chtot

				obj.Filename = filename

				fsize, err := strconv.ParseInt(filesize, 10, 64)
				if err != nil {
					return ParseResult{nil, onerror}, ErrParsingFailed.Wraps(err)
				}
				obj.Filesize = fsize

				chsize, err := strconv.ParseInt(chunksize, 10, 64)
				if err != nil {
					return ParseResult{nil, onerror}, ErrParsingFailed.Wraps(err)
				}
				obj.Size = chsize

				fileuuid, err := h.Session.Get(r.Context(), uploadid)
				if err != nil {
					return ParseResult{nil, onerror}, ErrUploadingFailed.Wraps(errors.New("Missing file UUID. Could not find in session for given uploadid. Upload complete or aborted."))
				}
				obj.FileUUID = string(fileuuid)

				if f[fieldIndex].upload == nil {
					return ParseResult{nil, onerror}, ErrServerFormInvalid.Wraps(errors.New("Field initialization error. Lacking the upload function."))
				}
				// upload
				n, cancel, err := f[fieldIndex].upload(r.Context(), obj)
				if err != nil {
					return ParseResult{nil, onerror}, err
				}
				onerror.Add(cancel)
				f[fieldIndex].Files = []Object{obj}
				uploadFileCreated = true
				if n == f[fieldIndex].SizeLimit {
					s := make([]byte, 1)
					c, _ := p.Read(s)
					if c != 0 {
						return ParseResult{nil, onerror}, ErrUploadTooLarge.Wraps(errors.New("Total upload size limited to: " + strconv.Itoa(int(f[fieldIndex].SizeLimit))))
					}
				}
			} else {
				var b *bytes.Buffer
				n, err := b.ReadFrom(pr)
				if err != nil {
					if err != io.EOF {
						return ParseResult{nil, onerror}, err
					}
				}
				if n == f[fieldIndex].SizeLimit {
					s := make([]byte, 1)
					c, _ := p.Read(s)
					if c != 0 {
						return ParseResult{nil, onerror}, ErrUploadTooLarge.Wraps(errors.New("Total upload size limited to: " + strconv.Itoa(int(f[fieldIndex].SizeLimit)))) // todo perhaps convey the limits back to the client
					}
				}
			}

			// Let's apply the validators
			ok, err := f[fieldIndex].IsValid()
			if !ok {
				return ParseResult{nil, onerror}, err
			}
		}
		if fieldIndex >= len(f) {
			return ParseResult{nil, onerror}, ErrClientFormInvalid.Wraps(errors.New("The submitted form has a field " + name + " which does not seem to be expected by the server."))
		}
		if _, err := reader.NextPart(); err != io.EOF {
			return ParseResult{nil, onerror}, ErrClientFormInvalid.Wraps(errors.New("The end of the submitted form does not seem to have been reached or the submitted form is badly formatted."))
		}
	}
	return ParseResult{f, onerror}, nil
}

type ChunkHandler struct {
	Handler
	Session session.Handler

	maxage         int
	maxConcurrency int
	bottleneck     *bottleneck.Client
}

// New returns a handler for a chunked upload request.
// An upload request starts by the creation of an upload session.
// By defauklt, the session remains valid for seven days
func Chunked(h Handler) ChunkHandler {
	uploadSessionHandler := h.Session.Spawn("uploads", session.SetMaxage(7*24*60*60), session.SetUUIDgenerator(h.FileIDgenerator), session.ServerOnly())
	// By default, the upload id generator is the the file uuid generator.
	return ChunkHandler{h, uploadSessionHandler, 7 * 24 * 60 * 60, 1, nil}
}

func (c ChunkHandler) Configure(functions ...func(ChunkHandler) ChunkHandler) ChunkHandler {
	for _, f := range functions {
		c = f(c)
	}
	return c
}

// SetSessionMaxAge sets the upload session maxage in seconds.
func SetSessionMaxAge(maxage int) func(ChunkHandler) ChunkHandler {
	return func(c ChunkHandler) ChunkHandler {
		c.Session = c.Session.Configure(session.SetMaxage(maxage))
		c.maxage = maxage
		return c
	}
}
func SetMaxConcurrency(n int, limiter *bottleneck.Client) func(ChunkHandler) ChunkHandler {
	return func(c ChunkHandler) ChunkHandler {
		c.maxConcurrency = n
		c.bottleneck = limiter
		return c
	}
}

func SetUploadIDgenerator(uuidFn func() (string, error)) func(ChunkHandler) ChunkHandler {
	return func(c ChunkHandler) ChunkHandler {
		c.Session = c.Session.Configure(session.SetUUIDgenerator(uuidFn))
		return c
	}
}

func (c ChunkHandler) Initializer() Initializer {
	return Initializer{&c, nil}
}

func (c ChunkHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx:= r.Context()
	// Parsing the form
	res, err := c.ParseUpload(w, r)
	if err != nil {

		err2 := res.Cancel()
		if c.Log != nil {
			c.Log.Print(err2)
		}

		switch err {
		case ErrNoBoundary, ErrBadContentType, ErrClientFormInvalid:
			http.Error(w, "Expecting correct form-data", http.StatusBadRequest)
			return
		case ErrParsingFailed, ErrUploadingFailed, ErrServerFormInvalid:
			http.Error(w, "Server was unable to proceed with request processing", http.StatusInternalServerError)
			return
		case ErrUploadTooLarge:
			http.Error(w, err.Error(), http.StatusRequestEntityTooLarge)
			return
		default:
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

	}
	ctx = context.WithValue(ctx, c.Handler.ctxKey, res)
	r = r.WithContext(ctx)
	if c.Handler.next != nil {
		c.Handler.next.ServeHTTP(w, r)
	}
}

func (c ChunkHandler) Link(hn xhttp.Handler) xhttp.HandlerLinker {
	c.Handler.next = hn
	return c
}

// Initializer handles chunked upload initialization request. It creates a new
// session upload whose id should be transmitted to the client to attach to each
// chunk information.
// The upload id generator that should be used can be further specified via the
// SetIDgenerator method.
type Initializer struct {
	c    *ChunkHandler
	next xhttp.Handler
}

func (i Initializer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
ctx:= r.Context()
	if !i.c.Handler.Session.Loaded(ctx) {
		http.Error(w, "User session does not seem to have been loaded", http.StatusUnauthorized)
		return
	}
	id, err := i.c.Handler.Session.ID()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if i.c.bottleneck != nil {
		err = i.c.bottleneck.NewBottleneck(id, i.c.maxage, i.c.maxConcurrency)
		if err != nil {
			http.Error(w, "Unable to reach upload permission server", http.StatusInternalServerError)
			return
		}
		t, err := i.c.bottleneck.NewTicket(id)
		if err != nil {
			http.Error(w, "Unable to request for upload permission", http.StatusInternalServerError)
			return
		}
		t, err = i.c.bottleneck.ExchangeTicket(id, t)
		if err != nil {
			http.Error(w, "Unable to request for upload permission", http.StatusInternalServerError)
			return
		}
		if !t.Winning() {
			http.Error(w, "The maximum number of concurrent uploads has been reached. Please, wait for an upload to complete and retry.", http.StatusTooManyRequests)
			return
		}

		// We can create a new upload session
		err = i.c.Session.Generate(w, r)
		if err != nil {
			http.Error(w, "Failed to generate new upload session", http.StatusInternalServerError)
			return
		}

		// this ticket needs to be stored as we need to try and return it
		b, err := t.Marshal()
		if err != nil {
			http.Error(w, "Unable to serialize upload permission for storage", http.StatusInternalServerError)
			return
		}
		err = i.c.Session.Put(ctx, TicketKey, b, 0) // TODO: do we need to set a maxage on this???
		if err != nil {
			http.Error(w, "Unable to serialize upload permission for storage", http.StatusInternalServerError)
			return
		}
	}

	// if session has not been generated before, i.e. no concurrency limiting is
	// implemented we generate a new session.
	if !i.c.Session.Loaded(ctx) {
		err = i.c.Session.Generate(w, r)
		if err != nil {
			http.Error(w, "Failed to generate new upload session", http.StatusInternalServerError)
			return
		}
	}

	uploadid, err := i.c.Session.ID()
	if err != nil {
		http.Error(w, "upload session seems to have been ill-instantiated. unable to retrieve upload session id.", http.StatusInternalServerError)
		if i.c.Handler.Log != nil {
			i.c.Handler.Log.Print(err)
		}
		return
	}
	// 1. either we manage to retrieve the upload session tied to the current navigation session
	// or we create a new upload session for the current navigation session
	//
	// 2. we have to get the rights to start a new upload by getting a ticket from the bottleneck service
	// so we try to create the bottleneck with the maxConcurrency setting  TODO decide if limit should be per user or per session
	// if per navigation session, the bottleneck is a bit less necessary
	// if per user, we need an active user session so that we can use the userid as id for the bottleneck.

	fileuuid, err := i.c.FileIDgenerator()
	if err != nil {
		http.Error(w, "Failed to generate file uuid.", http.StatusInternalServerError)
		if i.c.Handler.Log != nil {
			i.c.Handler.Log.Print(err)
		}
		return
	}

	err = i.c.Session.Put(ctx, uploadid, []byte(fileuuid), 0)
	if err != nil {
		http.Error(w, "Failed to link into session upload id and file UUID", http.StatusInternalServerError)
		if i.c.Handler.Log != nil {
			i.c.Handler.Log.Print(err)
		}
		return
	}

	err = i.c.Session.Save( w, r)
	if err != nil {
		http.Error(w, "Unable to set upload session cookie", http.StatusInternalServerError)
		if i.c.Handler.Log != nil {
			i.c.Handler.Log.Print(err)
		}
		return
	}

	w.Write([]byte(uploadid))

	r = r.WithContext(ctx)
	if i.c.next != nil {
		i.c.next.ServeHTTP(w, r)
	}
}

func (i Initializer) Link(h xhttp.HandlerLinker) xhttp.Handler {
	i.next = h
	return i
}
