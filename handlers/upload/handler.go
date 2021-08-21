package upload

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"log"
	"mime"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/atdiar/errors"
	"github.com/atdiar/xhttp"
	"github.com/atdiar/xhttp/handlers/session"
)

// NOTE Do not use on 32bit platforms or anywhere where int size is below int64

var (
	AudioMIMETypes = newSet().Add("audio/mpeg", "audio/ogg", "audio/wav", "audio/webm", "audio/basic", "audio/aiff", "audio/midi", "audio/wave")
	VideoMIMETypes = newSet().Add("video/avi", "video/mpeg", "video/ogg", "video/webm", "video/mp4")
)

var (
	ErrUploadTooLarge    = errors.New("Upload too large")
	ErrNoBoundary        = errors.New("Unable to parse submitted form. Missing boundary.")
	ErrServerFormInvalid = errors.New("Server Error: upload form is invalid.")
	ErrClientFormInvalid = errors.New("Client Error: submitted upload form is invalid.")
	ErrParsingFailed     = errors.New("Failed to parse form.")
	ErrBadContentType    = errors.New("Unsupported content type.")
	ErrUploadingFailed   = errors.New("File uploading failed")
)

// Path is a utility function used to create upload storage path and s3 keys.
func Path(strings ...string) string {
	var s string
	for _, str := range strings {
		s = s + "/" + str
	}
	return s
}

type contextKey struct{}

// Form is a type that can be used to represent the  structure of a
// form upload as expected by the server. The server would parse a POST or PUT
// form-data upload and validate it. Any file is stream uploaded to its end
// storage thanks to a provided uploading function specified when creating the
// expected FileField.
type Form []Field

// NewForm returns an upload form specification used when parsing a form upload request.
func NewForm(fields ...Field) Form {
	return fields
}

// Get returns the vraw value sent for the (non-file) form field of the given name.
// It returns a non nil error if the field cannot be found after parsing.
func (f Form) Get(fieldname string) (val []byte, err error) {
	for _, field := range f {
		if fieldname != field.Name {
			continue
		}
		if field.Files != nil {
			return val, errors.New("This is a file upload field, not a regular field. Unable to retrieve value.")
		}
		val = field.Body
		break
	}
	return val, err
}

// ParseUpload parses a submitted form-data POST or PUT request, uploading any submitted
// file within the limits defined for the endpoint in terms of upload size.
func (h Handler) ParseUpload(w http.ResponseWriter, r *http.Request) (ParseResult, error) {
	onerror := newCanceler()
	f := h.Form
	// Let's get the uploader id
	err := h.Session.Load(w, r)
	if err != nil {
		return ParseResult{nil, onerror}, ErrParsingFailed.Wraps(errors.New("Unable to load session").Wraps(err))
	}
	uploaderid, err := h.Session.ID()
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

	reader := multipart.NewReader(r.Body, params["boundary"])
	if len(f) == 0 {
		return ParseResult{f, onerror}, ErrServerFormInvalid
	}
	for fieldIndex := 0; fieldIndex < len(f); fieldIndex++ {
		p, err := reader.NextPart()
		if err != nil {
			if err != io.EOF {
				return ParseResult{f, onerror}, ErrParsingFailed.Wraps(err)
			}
			for j := fieldIndex; j < len(f); j++ {
				if !f[fieldIndex].Required {
					continue
				} else {
					return ParseResult{f, onerror}, ErrClientFormInvalid.Wraps(errors.New("upload form sent is missing a required field: " + f[fieldIndex].Name))
				}
			}
			return ParseResult{f, onerror}, nil
		}

		contentDisposition, _, _ := mime.ParseMediaType(p.Header.Get("Content-Disposition"))
		if contentDisposition != "form-data" {
			return ParseResult{f, onerror}, ErrClientFormInvalid.Wraps(errors.New("Submitted form has bad formatting. Expecting Content-Disposition Header to be form-data for each part."))
		}

		name := p.FormName()
		filenameIfExists := p.FileName()

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
			contentType, params2, err := mime.ParseMediaType(p.Header.Get("Content-Type"))
			if err != nil {
				buf := bufio.NewReader(p)
				peeksize := 512
				if f[fieldIndex].SizeLimit < int64(peeksize) {
					peeksize = int(f[fieldIndex].SizeLimit)
				}
				sniff, _ := buf.Peek(peeksize)
				contentType = http.DetectContentType(sniff)
				if !f[fieldIndex].AllowedContentTypes.Contains(contentType, false) {
					if filenameIfExists != "" {
						ext := filepath.Ext(filenameIfExists)
						if ext != "" {
							contentType = mime.TypeByExtension(ext)
							if !f[fieldIndex].AllowedContentTypes.Contains(contentType, false) {
								return ParseResult{f, onerror}, errors.New("Unknown Content-Type")
							}
						}
						return ParseResult{f, onerror}, errors.New("Unsupported Content-Type")
					}
					return ParseResult{f, onerror}, errors.New("Unsupported Content-Type")
				}
				f[fieldIndex].ContentType = contentType
			}
			if !f[fieldIndex].AllowedContentTypes.Contains(contentType, false) {
				return ParseResult{f, onerror}, ErrClientFormInvalid.Wraps(ErrBadContentType)
			}
			f[fieldIndex].ContentType = contentType

			// Let's retrieve the data and make sure it fits within the size limit
			// If the data is of content-type multipart/mixed, it means it is a
			// multipart message comprised of different files.
			if contentType == "multipart/mixed" {
				if _, ok := params2["boundary"]; !ok {
					return ParseResult{f, onerror}, ErrParsingFailed.Wraps(ErrNoBoundary)
				}
				freader := multipart.NewReader(p, params2["boundary"])
				//filecount := 0
				remainingSize := f[fieldIndex].SizeLimit

				for {
					q, err := freader.NextPart()
					if err != nil {
						if err == io.EOF {
							break
						}
						return ParseResult{nil, onerror}, ErrParsingFailed.Wraps(err)
					}
					// Get file content-type
					ct, _, err := mime.ParseMediaType(q.Header.Get("Content-Type"))
					if err != nil {
						buf := bufio.NewReader(p)
						peeksize := 512
						if remainingSize < int64(peeksize) {
							peeksize = int(remainingSize)
						}
						sniff, _ := buf.Peek(peeksize)
						ct = http.DetectContentType(sniff)
					}
					// See if the content-type is supported
					if !f[fieldIndex].AllowedContentTypes.Contains(contentType, false) || ct == "multipart/mixed" {
						return ParseResult{nil, onerror}, ErrBadContentType
					}
					// create a new file , populate it, and add it to the filelist

					obj := NewFile(io.LimitReader(q, remainingSize), q.FileName(), ct, uploaderid, f[fieldIndex].Path)
					id, err := h.FileIDgenerator()
					if err != nil {
						return ParseResult{nil, onerror}, ErrUploadingFailed.Wraps(errors.New("Unable to generate unique id for the upload file. Operation aborted")) // todo see if we could just skip the failing parts and retry perhaps
					}
					obj.FileUUID = id
					if f[fieldIndex].upload == nil {
						return ParseResult{nil, onerror}, ErrServerFormInvalid.Wraps(errors.New("Field initialization error. Lacking the upload function."))
					}
					// upload
					n, cancel, err := f[fieldIndex].upload(r.Context(), obj) // todo cancel function needs to be saved somewhere like to)p level slice of cancelfunction
					if err != nil {
						return ParseResult{nil, onerror}, ErrUploadingFailed.Wraps(err)
					}
					onerror.Add(cancel)

					f[fieldIndex].Files = append(f[fieldIndex].Files, obj)

					remainingSize -= n
					if remainingSize < 0 {
						return ParseResult{nil, onerror}, ErrUploadTooLarge.Wraps(errors.New("Total upload size limited to: " + strconv.Itoa(int(f[fieldIndex].SizeLimit))))
					}
					s := make([]byte, 1)
					c, _ := q.Read(s)
					if c != 0 {
						return ParseResult{nil, onerror}, ErrUploadTooLarge.Wraps(errors.New("Total upload size limited to: " + strconv.Itoa(int(f[fieldIndex].SizeLimit))))
					}
				}
			} else {
				pr := io.LimitReader(p, f[fieldIndex].SizeLimit)
				if f[fieldIndex].Files != nil {
					obj := NewFile(pr, filenameIfExists, contentType, uploaderid, f[fieldIndex].Path)
					id, err := h.FileIDgenerator()
					if err != nil {
						return ParseResult{nil, onerror}, ErrUploadingFailed.Wraps(errors.New("Unable to generate unique id for the upload file. Operation aborted"))
					}
					obj.FileUUID = id
					if f[fieldIndex].upload == nil {
						return ParseResult{nil, onerror}, ErrServerFormInvalid.Wraps(errors.New("Field initialization error. Lacking the upload function."))
					}
					// upload
					n, cancel, err := f[fieldIndex].upload(r.Context(), obj) // todo cancel function needs to be saved somewhere like to)p level slice of cancelfunction
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

// ParseResult holds the results from parsing a form upload request.
// It holds the form filled from the parsed data and a ffunction that can be used
// to try and  rollback the file uploads. (for instance in case registering the
// file data in the database failed, one could decide to rollback the file storage)
// Canceling/rolling back an upload should be idemptotent.
// Means that each file uploads returning a cancelation function should return
// an idempotent one.
type ParseResult struct {
	Form Form
	*canceler
}

type canceler struct {
	funcList []func() error
}

func newCanceler() *canceler {
	return &canceler{make([]func() error, 1)}
}

func (c *canceler) Add(cancelFn ...func() error) {
	c.funcList = append(c.funcList, cancelFn...)
}

func (c *canceler) Cancel() error {
	l := errors.NewList()
	for _, f := range c.funcList {
		err := f()
		if err != nil {
			l.Add(err)
		}
	}
	if l.Nil() {
		return nil
	}
	return l
}

// Field is a type used to define the structure of a form field.
type Field struct {
	Name        string
	Body        []byte
	ContentType string

	Path   string
	Files  FileList
	upload func(context.Context, Object) (int64, func() error, error)

	AllowedContentTypes set
	SizeLimit           int64
	Required            bool

	Validators []func(Field) (bool, error)
}

func (f Field) expectFile() bool {
	return f.Files != nil
}

type FileList []Object

func (f FileList) Size() int64 {
	var count int64
	for _, file := range f {
		//not checking for overflow or trim at max int64 value because it's not
		//  realistic. Besides the post body will be limited in size
		count += file.Size
	}
	return count
}

// NewField is used to create the specification for a data form field with  that
// the client request should adhere to.
func NewField(name string, sizelimit int, required bool, AcceptedContentTypes ...string) Field {
	return Field{name, nil, "", "", nil, nil, newSet().Add(AcceptedContentTypes...), int64(sizelimit), required, nil}
}

// NewFileField is used to create the specification for a file upload form field
//  with constraints that the client should adhere to and that the request parser
// will verify.
func NewFileField(name string, sizelimit int, required bool, multiple bool, storagepath string, uploadFn func(context.Context, Object) (bytesuploaded int64, rollbackFn func() error, err error), AcceptedContentTypes ...string) Field {
	var l int
	act := newSet().Add(AcceptedContentTypes...)
	if multiple {
		l = 2
		act = act.Add("multipart/mixed")
	}
	return Field{name, nil, "", storagepath, FileList(make([]Object, l)), uploadFn, act, int64(sizelimit), required, nil}
}

// Validators register validatiog functions for a form field .
func (f Field) Validator(v ...func(Field) (bool, error)) Field {
	f.Validators = v
	return f
}

// IsValid rettur,s the validity of a submitted form field with an accompanying
// explanatory error in case of failure.
func (f Field) IsValid() (bool, error) {
	for _, v := range f.Validators {
		if b, err := v(f); !b {
			return b, err
		}
	}
	return true, nil
}

// Object is a structured representation for an upload file and its metadata.
type Object struct {
	UploadID   string // can be created by the upload process/function.
	UploaderID string
	Size       int64 // object size : if not chunked, Size = FileSize

	ChunkOffset int64
	ChunksTotal int64

	Filename string // If file name is absent, it shpould be replace by FileUUID
	Filesize int64
	FileUUID string // server-generated
	Path     string

	ContentType string
	Binary      io.Reader
}

// EvalPath replaces the placeholder strings starting by '%' with their respective
// value as stored in the Object type variable.
func (o Object) EvalPath() string {
	p := strings.ReplaceAll(o.Path, "%uploadid", o.UploadID)
	p = strings.ReplaceAll(p, "%uploaderid", o.UploaderID)
	p = strings.ReplaceAll(p, "%chunkoffset", strconv.FormatInt(o.ChunkOffset, 10)) // not expected to be in use
	p = strings.ReplaceAll(p, "%filename", o.Filename)                              // not expected to be in use
	return p
}

// NewFile creates a new upload.Object used to hold uploading information as well as
// upload data accessible via an io.Reader.
// The accompanying  upload object info can be stored in the database once the
// data has been successfully uploaded.
func NewFile(src io.Reader, filename string, contenttype string, uploaderID string, uploadpath string) Object {
	o := Object{}
	o.Binary = src
	o.Filename = filename
	o.ContentType = contenttype
	o.UploaderID = uploaderID
	o.Path = uploadpath
	return o
}

// Handler handles http upload requests, verifying that the request implements the
// specification of the upload.Form.
type Handler struct {
	Form    Form
	Session session.Handler // used to retrieve the session id

	Path string

	FileIDgenerator func() (string, error) // used to generate a file unique identifier

	Log *log.Logger

	ctxKey contextKey

	next xhttp.Handler
}

// New returns a http request handler that will parse a request in order to
// try and retrieve values if the structure of the request fits the expected
// model defined in an upload Form.
func New(f Form, s session.Handler, uploadpath string, fileUUIDgenerator func() (string, error)) Handler {
	return Handler{f, s, uploadpath, fileUUIDgenerator, nil, contextKey{}, nil}
}

// WithLogger enables logging capabilities. Typically for logging errors. such as
// a failure to rollback an upload even though the parsing failed because the
// submitted form requets is malformed.
func (h Handler) WithLogger(l *log.Logger) Handler {
	h.Log = l
	return h
}

func (h Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	// Limit size of the request
	//r.Body = http.MaxBytesReader(w, r.Body, h.ReqMaxSize)

	// Parsing the form
	res, err := h.ParseUpload(w, r)
	if err != nil {
		if h.Log != nil {
			h.Log.Print(err)
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
	ctx = context.WithValue(ctx, h.ctxKey, res)
	r = r.WithContext(ctx)
	if h.next != nil {
		h.next.ServeHTTP(w, r)
	}
}

func (h Handler) Link(hn xhttp.Handler) xhttp.HandlerLinker {
	h.next = hn
	return h
}

// ParseResults attempts to retrieve the results obtained after an upload request
// has been parsed.
func ParseResults(ctx context.Context) (ParseResult, bool) {
	p, ok := ctx.Value(contextKey{}).(ParseResult)
	return p, ok
}

// set defines an unordered list of string elements.
// Two methods have been made available:
// - an insert method called `Add`
// - a delete method called `Remove`
// - a lookup method called `Contains`
type set map[string]bool

func newSet() set {
	s := make(map[string]bool)
	return s
}

func (s set) Add(strls ...string) set {
	for _, str := range strls {
		s[str] = true
	}
	return s
}

func (s set) Remove(str string, caseSensitive bool) {
	if !caseSensitive {
		str = strings.ToLower(str)
	}
	delete(s, str)
}

func (s set) Contains(str string, caseSensitive bool) bool {
	if !caseSensitive {
		str = strings.ToLower(str)
	}
	return s[str]
}

func (s set) Includes(v set) bool {
	for k := range v {
		if !s[k] {
			return false
		}
	}
	return true
}

func (s set) Union(v set) set {
	r := newSet()
	for k := range v {
		r.Add(k)
	}
	for k := range s {
		r.Add(k)
	}
	return r
}

func (s set) Inter(v set) set {
	r := newSet()
	for k := range v {
		if s[k] {
			r.Add(k)
		}
	}
	return r
}

func (s set) List() []string {
	l := make([]string, len(s))
	i := 0
	for k := range s {
		l[i] = k
		i++
	}
	return l
}

func (s set) Count() int {
	return len(s)
}
