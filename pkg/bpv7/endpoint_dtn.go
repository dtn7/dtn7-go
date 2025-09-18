// SPDX-FileCopyrightText: 2020 Alvar Penning
//
// SPDX-License-Identifier: GPL-3.0-or-later

package bpv7

import (
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/dtn7/cboring"
)

const (
	DtnEndpointSchemeName = "dtn"
	DtnEndpointSchemeNo   = uint64(1)
	DtnEndpointDtnNone    = "dtn:none"
	DtnEndpointDtnNoneSsp = "none"

	DtnEndpointRegexpSsp     = `//((^$)|([\w-._~!$&'()*+,;=]+))/(.*)`
	DtnEndpointRegexpFull    = "^" + DtnEndpointSchemeName + ":(none|" + DtnEndpointRegexpSsp + ")$"
	DtnEndpointRegexpNotNone = "^" + DtnEndpointSchemeName + ":(" + DtnEndpointRegexpSsp + ")$"
)

// DtnEndpoint describes the dtn URI for EndpointIDs, as defined in RFC 9171.
//
//	Format of a "normal" dtn URI:
//	"dtn:" "//" NodeName "/" Demux
//	            ^------^ 1*(ALPHA/DIGIT/"-"/"."/"_")
//	                         ^---^ *VCHAR
//
//	Format of the null endpoint:
//	"dtn:none"
type DtnEndpoint struct {
	NodeName string
	Demux    string

	IsDtnNone bool
}

// parseDtnSsp tries to parse a "dtn" URI's scheme specific part (SSP) and return the URI's parts.
func parseDtnSsp(ssp string) (nodeName, demux string, isDtnNone bool, err error) {
	// As defined in RFC 9171, a "dtn" URI might be the null endpoint "dtn:none" or something URI/IRI like.
	// Thus, at first we are going after the null endpoint and inspect a more generic URI afterwards.

	if ssp == DtnEndpointDtnNoneSsp {
		isDtnNone = true
		return
	}

	re := regexp.MustCompile("^" + DtnEndpointRegexpSsp + "$")
	if !re.MatchString(ssp) {
		err = fmt.Errorf("ssp does not match a dtn endpoint")
		return
	}

	ssp = ssp[2:]

	parts := strings.SplitN(ssp, "/", 2)
	nodeName = parts[0]
	demux = parts[1]
	return
}

// NewDtnEndpoint from an URI with the dtn scheme.
func NewDtnEndpoint(uri string) (e EndpointType, err error) {
	if !strings.HasPrefix(uri, DtnEndpointSchemeName+":") {
		err = fmt.Errorf("URI does not start with the \"dtn\" URI prefix (\"dtn:\")")
		return
	}

	if nodeName, demux, isDtnNode, parseErr := parseDtnSsp(uri[len(DtnEndpointSchemeName)+1:]); parseErr != nil {
		err = parseErr
		return
	} else if isDtnNode {
		e = DtnEndpoint{IsDtnNone: true}
	} else {
		e = DtnEndpoint{
			NodeName:  nodeName,
			Demux:     demux,
			IsDtnNone: false,
		}
	}

	err = e.CheckValid()
	return
}

// SchemeName is "dtn" for DtnEndpoints.
func (_ DtnEndpoint) SchemeName() string {
	return DtnEndpointSchemeName
}

// SchemeNo is 1 for DtnEndpoints.
func (_ DtnEndpoint) SchemeNo() uint64 {
	return DtnEndpointSchemeNo
}

// Authority is the authority part of the Endpoint URI, e.g., "foo" for "dtn://foo/bar" or "none" for "dtn:none".
func (e DtnEndpoint) Authority() string {
	if e.IsDtnNone {
		return DtnEndpointDtnNoneSsp
	} else {
		return e.NodeName
	}
}

// Path is the path part of the Endpoint URI, e.g., "/bar" for "dtn://foo/bar" or "/" for "dtn:none".
func (e DtnEndpoint) Path() string {
	if e.IsDtnNone {
		return "/"
	} else {
		return "/" + e.Demux
	}
}

// IsSingleton checks if this Endpoint represents a singleton.
//
// - If a "dtn" URI's demux start with "~", this Endpoint is not a singleton.
// - "dtn:none" cannot be a singleton.
func (e DtnEndpoint) IsSingleton() bool {
	return !strings.HasPrefix(e.Demux, "~") && !e.IsDtnNone
}

// CheckValid returns an error for incorrect data.
func (e DtnEndpoint) CheckValid() (err error) {
	if e.IsDtnNone {
		return
	}
	ssp := fmt.Sprintf("//%v/%v", e.NodeName, e.Demux)
	if !regexp.MustCompile(DtnEndpointRegexpSsp).MatchString(ssp) {
		err = fmt.Errorf("malformed ssp: %v", ssp)
	}
	return
}

func (e DtnEndpoint) String() string {
	if e.IsDtnNone {
		return DtnEndpointDtnNone
	} else {
		return fmt.Sprintf("%s://%s/%s", DtnEndpointSchemeName, e.NodeName, e.Demux)
	}
}

// MarshalCbor writes this DtnEndpoint's CBOR representation.
func (e DtnEndpoint) MarshalCbor(w io.Writer) error {
	if e.IsDtnNone {
		return cboring.WriteUInt(0, w)
	} else {
		ssp := fmt.Sprintf("//%s/%s", e.NodeName, e.Demux)
		return cboring.WriteTextString(ssp, w)
	}
}

// UnmarshalCbor reads a CBOR representation.
func (e *DtnEndpoint) UnmarshalCbor(r io.Reader) error {
	if m, n, err := cboring.ReadMajors(r); err != nil {
		return err
	} else {
		switch m {
		case cboring.UInt:
			// dtn:none
			e.IsDtnNone = true

		case cboring.TextString:
			// dtn://node-name/[demux]
			if ssp, err := cboring.ReadRawBytes(n, r); err != nil {
				return err
			} else if nodeName, demux, isDtnNode, parseErr := parseDtnSsp(string(ssp)); parseErr != nil {
				return parseErr
			} else if isDtnNode {
				return fmt.Errorf("DtnEndpoint: byte based SSP represents \"dtn:none\"")
			} else {
				e.NodeName = nodeName
				e.Demux = demux
				e.IsDtnNone = false
			}

		default:
			return fmt.Errorf("DtnEndpoint: wrong major type 0x%X for unmarshalling", m)
		}
	}

	return nil
}

func (e DtnEndpoint) IsNone() bool {
	return e.IsDtnNone
}

// DtnNone returns a new instance of the null endpoint "dtn:none".
func DtnNone() EndpointID {
	return EndpointID{DtnEndpoint{IsDtnNone: true}}
}
