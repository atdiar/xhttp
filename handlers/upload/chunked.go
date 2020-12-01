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

	"github.com/atdiar/errors"
	"github.com/atdiar/xhttp"
	"github.com/atdiar/xhttp/handlers/session"
)

var (
	FileNameFieldName    = "filename"
	FileSizeFieldName    = "filesize"
	UploadIDFieldName    = "uploadid"
	ChunkOffsetFieldName = "chunkoffset"
	ChunksTotalFieldName = "chunkstotal"
	ChunkSizeFieldName   = "chunksize"
)

// ParseUpload parses a submitted form-data POST or PUT request, uploading any submitted
// file within the limits defined for the endpoint in terms of upload size.
func (h ChunkHandler) ParseUpload(ctx context.Context, w http.ResponseWriter, r *http.Request) (ParseResult, error) {
	onerror := newCanceler()
	f := h.Handler.Form
	// Let's get the uploader id
	ctx, err := h.Handler.Session.Load(ctx, w, r)
	if err != nil {
		return ParseResult{nil, onerror}, ErrParsingFailed.Wraps(errors.New("Unable to load session").Wraps(err))
	}
	uploaderid, err := h.Handler.Session.ID()
	if err != nil {
		return ParseResult{nil, onerror}, ErrParsingFailed.Wraps(errors.New("No session ID found. Unable to retrieve uploader session id").Wraps(err))
	}

	contentType, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || !strings.HasPrefix(contentType, "multipart/") {
		return ParseResult{f, onerror}, errors.New("Content-Type error : expecting a multipart message")
	}
	if _, ok := params["boundary"]; !ok {
		return ParseResult{f, onerror}, ErrNoBoundary
	}

	// The following are values for the chunk upload that are initially nil and
	// should only be modified once as only one file should be uploaded at a time.
	var (
		uploadid    []byte
		filename    []byte
		filesize    []byte
		chunksize   []byte
		chunkoffset []byte
		chunkstotal []byte
	)

	reader := multipart.NewReader(r.Body, params["boundary"])
	if len(f) == 0 {
		return ParseResult{f, onerror}, ErrServerFormInvalid
	}
	for fieldIndex := 0; fieldIndex < len(f); fieldIndex++ {
		p, err := reader.NextPart()
		if err != nil {
			if err != io.EOF {
				if err := onerror.Cancel(); err != nil {
					if h.Handler.Log != nil {
						h.Handler.Log.Print(err)
					}
				}
				return ParseResult{f, onerror}, ErrParsingFailed.Wraps(err)
			}
			for j := fieldIndex; j < len(f); j++ {
				if !f[fieldIndex].Required {
					continue
				} else {
					if err := onerror.Cancel(); err != nil {
						if h.Handler.Log != nil {
							h.Handler.Log.Print(err)
						}
					}
					return ParseResult{f, onerror}, ErrClientFormInvalid.Wraps(errors.New("upload form sent is missing a required field: " + f[fieldIndex].Name))
				}
			}
			return ParseResult{f, onerror}, nil
		}

		contentDisposition, _, _ := mime.ParseMediaType(p.Header.Get("Content-Disposition"))
		if contentDisposition != "form-data" {
			if err := onerror.Cancel(); err != nil {
				if h.Handler.Log != nil {
					h.Handler.Log.Print(err)
				}
			}
			return ParseResult{f, onerror}, ErrClientFormInvalid.Wraps(errors.New("Submitted form has bad formatting. Expecting Content-Disposition Header to be form-data for each part."))
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
				if err := onerror.Cancel(); err != nil {
					if h.Handler.Log != nil {
						h.Handler.Log.Print(err)
					}
				}
				return ParseResult{f, onerror}, ErrClientFormInvalid.Wraps(ErrBadContentType)
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
				if uploadid != nil || filename != nil || filesize != nil || chunksize != nil || chunkoffset != nil || chunkstotal != nil {
					return ParseResult{nil, onerror}, ErrServerFormInvalid.Wraps(errors.New("Form is malformed server side. Only one file upload field is allowed for chunk uploads"))
				}
				uploadid, err = h.Handler.Form.Get(UploadIDFieldName)
				if err != nil {
					return ParseResult{nil, onerror}, ErrClientFormInvalid.Wraps(errors.New(UploadIDFieldName + " field is necessary for a chunk upload."))
				}
				filename, err = h.Handler.Form.Get(FileNameFieldName)
				if err != nil {
					return ParseResult{nil, onerror}, ErrClientFormInvalid.Wraps(errors.New(FileNameFieldName + " field is necessary for a chunk upload."))
				}
				filesize, err = h.Handler.Form.Get(FileSizeFieldName)
				if err != nil {
					return ParseResult{nil, onerror}, ErrClientFormInvalid.Wraps(errors.New(FileSizeFieldName + " field is necessary for a chunk upload."))
				}
				chunksize, err = h.Handler.Form.Get(ChunkSizeFieldName)
				if err != nil {
					return ParseResult{nil, onerror}, ErrClientFormInvalid.Wraps(errors.New(ChunkSizeFieldName + " field is necessary for a chunk upload."))
				}
				chunkoffset, err = h.Handler.Form.Get(ChunkOffsetFieldName)
				if err != nil {
					return ParseResult{nil, onerror}, ErrClientFormInvalid.Wraps(errors.New(ChunkOffsetFieldName + " field is necessary for a chunk upload."))
				}
				chunkstotal, err = h.Handler.Form.Get(ChunksTotalFieldName)
				if err != nil {
					return ParseResult{nil, onerror}, ErrClientFormInvalid.Wraps(errors.New(ChunksTotalFieldName + " field is necessary for a chunk upload."))
				}

				obj := NewFile(pr, string(filename), contentType, uploaderid, f[fieldIndex].Path)

				obj.UploadID, err = h.Session.ID() //string(uploadid)
				if err != nil {
					return ParseResult{nil, onerror}, ErrUploadingFailed.Wraps(errors.New("Unable to retrieve upload if from upload session. Session may have been terminated or expired."))
				}
				if obj.UploadID != string(uploadid) {
					return ParseResult{nil, onerror}, ErrClientFormInvalid.Wraps(errors.New("The submitted uploadid value does not coincide with the upload session id."))
				}
				// TODO abort upload ?

				choff, err := strconv.ParseInt(string(chunkoffset), 10, 64)
				if err != nil {
					return ParseResult{nil, onerror}, ErrParsingFailed.Wraps(err)
				}
				obj.ChunkOffset = choff
				chtot, err := strconv.ParseInt(string(chunkstotal), 10, 64)
				if err != nil {
					return ParseResult{nil, onerror}, ErrParsingFailed.Wraps(err)
				}
				obj.ChunksTotal = chtot
				obj.Filename = string(filename)
				fsize, err := strconv.ParseInt(string(filesize), 10, 64)
				if err != nil {
					return ParseResult{nil, onerror}, ErrParsingFailed.Wraps(err)
				}
				obj.Filesize = fsize

				fileuuid, err := h.Session.Get(obj.UploadID)
				if err != nil {
					return ParseResult{nil, onerror}, ErrUploadingFailed.Wraps(errors.New("Missing file UUID. Could not find in session for given uploadid. Upload complete or aborted."))
				}
				obj.FileUUID = string(fileuuid)

				if f[fieldIndex].upload == nil {
					return ParseResult{nil, onerror}, ErrServerFormInvalid.Wraps(errors.New("Field initialization error. Lacking the upload function."))
				}
				// upload
				n, cancel, err := f[fieldIndex].upload(ctx, obj) // todo cancel function needs to be saved somewhere like to)p level slice of cancelfunction
				if err != nil {
					return ParseResult{nil, onerror}, err
				}
				onerror.Add(cancel)
				f[fieldIndex].Files = []Object{obj}
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
				if err := onerror.Cancel(); err != nil {
					if h.Handler.Log != nil {
						h.Handler.Log.Print(err)
					}
				}
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
}

// New returns a handler for a chunked upload request.
// An upload request starts by the creation of an upload session.
// By defauklt, the session remains valid for seven days
func Chunked(h Handler) ChunkHandler {
	uploadSessionHandler := session.New("upload", h.Session.Secret).Configure(session.SetMaxage(7*24*60*60), session.SetUUIDgenerator(h.IDgenerator))
	// By default, the upload id generator is the the file uuid generator.
	return ChunkHandler{h, uploadSessionHandler}
}

// SetSessionMaxAge sets the upload session maxage.
func (c ChunkHandler) SetSessionMaxAge(maxage int) ChunkHandler {
	c.Session = c.Session.Configure(session.SetMaxage(maxage))
	return c
}

func (c ChunkHandler) UploadInitializer() Initializer {
	return Initializer{&c, nil}
}

func (c ChunkHandler) ServeHTTP(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	// Try loading  upload session  to make sure we can start processing the
	// request for chunk upload
	ctx, err := c.Session.Load(ctx, w, r)
	if err != nil {
		http.Error(w, "Unable to load upload session: session expired or aborted", http.StatusUnauthorized)
		return
	}

	// Parsing the form
	res, err := c.ParseUpload(ctx, w, r)
	if err != nil {
		if c.Log != nil {
			c.Log.Print(err)
		}
		// todo switch on error value
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
		c.Handler.next.ServeHTTP(ctx, w, r)
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

func (i Initializer) SetIDgenerator(f func() (string, error)) Initializer {
	i.c.Session = i.c.Session.Configure(session.SetUUIDgenerator(f))
	return i
}

func (i Initializer) ServeHTTP(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	ctx, err := i.c.Session.Load(ctx, w, r)
	if err == nil {
		http.Error(w, "Concurrent uploads are not supported.", http.StatusBadRequest)
		return
	}

	ctx, err = i.c.Session.Generate(ctx, w, r)
	if err != nil {
		http.Error(w, "Failed to generate upload session.", http.StatusInternalServerError)
		if i.c.Handler.Log != nil {
			i.c.Handler.Log.Print(err)
		}
		return
	}

	fileuuid, err := i.c.IDgenerator()
	if err != nil {
		http.Error(w, "Failed to generate file uuid.", http.StatusInternalServerError)
		if i.c.Handler.Log != nil {
			i.c.Handler.Log.Print(err)
		}
		return
	}
	uploadid, err := i.c.Session.ID()
	if err != nil {
		http.Error(w, "upload session seems to have been ill-instantiated. unable to find upload session id.", http.StatusInternalServerError)
		if i.c.Handler.Log != nil {
			i.c.Handler.Log.Print(err)
		}
		return
	}
	err = i.c.Session.Put(uploadid, []byte(fileuuid), 0)
	if err != nil {
		http.Error(w, "Failed to link into session upload id and file UUID", http.StatusInternalServerError)
		if i.c.Handler.Log != nil {
			i.c.Handler.Log.Print(err)
		}
		return
	}

	w.Write([]byte(uploadid))

	if i.c.next != nil {
		i.c.next.ServeHTTP(ctx, w, r)
	}
}

func (i Initializer) Link(h xhttp.HandlerLinker) xhttp.Handler {
	i.next = h
	return i
}
