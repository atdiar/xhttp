package csrf

import (
	"errors"
	"github.com/iambase/gombo/middleware/session"
	"github.com/iambase/gombo/middleware/session/cookie"
	"github.com/iambase/gombo/router/context"
	"net/http"
	"sync/atomic"
	"unsafe"
)

type Inode struct {
	*csrf
	pad [128]byte
}

// New returns a new store Inode
func New(s *session.Inode) (*Inode, error) {
	if s == nil {
		return nil, errors.New("Link to Session inode is missing. Check parameter.")
	}
	n := new(Inode)
	c, err := newcsrf(s)
	if err != nil {
		return nil, err
	}
	n.csrf = c
	s.Configure(session.Options.Link(n))
	return n, nil
}

// Configure is used to modify an existing Inode.
func (i *Inode) Configure(options ...Option) error {
	vrplc := csrf{}
	vrplc = *(i.GetRaw())
	rplc := &vrplc
	var err error

	for _, opt := range options {
		err = opt(rplc)
		if err != nil {
			return err
		}
	}
	atomic.StorePointer((*unsafe.Pointer)(unsafe.Pointer(i.csrf)), unsafe.Pointer(&rplc))
	return nil
}

// Options is an exported object whose method is the constructor function of the Option object used to change the name of a *csrf object :
// Name(string), Session(*session.Inode)
var Options OptionList

func init() {
	Options.ChangeCookie = cookie.Options
}

func (i *Inode) AtomicREAD() unsafe.Pointer {
	return atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&i.csrf)))
}

func (i *Inode) GetRaw() *csrf {
	return (*csrf)(i.AtomicREAD())
}

// Exposed API

func (i *Inode) ServeHTTP(res http.ResponseWriter, req *http.Request, data *context.Instance) (http.ResponseWriter, bool) {
	c := i.GetRaw()
	return c.ServeHTTP(res, req, data)
}

func (i *Inode) Update() {
	c := i.GetRaw()
	c.Update()
}
