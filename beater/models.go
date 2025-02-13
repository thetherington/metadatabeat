package beater

import (
	"encoding/xml"
	"fmt"
	"reflect"
)

// MdseventType ...
type MdseventType struct {
	XMLName xml.Name `xml:"ev"`
	AID     string   `xml:"aID,attr,omitempty"       es:"asset_id"`
	Bc      string   `xml:"bc,attr"                  es:"bus_code"`
	Q       uint32   `xml:"q,attr"                   es:"queue_depth"`
	Pts     string   `xml:"pts,attr"                 es:"pts"`
	SrcT    string   `xml:"srcT,attr"                es:"source_type"`
	ConT    string   `xml:"conT,attr"                es:"content_type"`
	Art     string   `xml:"art,attr,omitempty"       es:"artist"`
	Titl    string   `xml:"titl,attr,omitempty"      es:"title"`
	Pid     string   `xml:"pid,attr,omitempty"       es:"pid"`
	Com1    string   `xml:"com1,attr,omitempty"      es:"comment_1"`
	Com2    string   `xml:"com2,attr,omitempty"      es:"comment_2"`
	ArtID   string   `xml:"artID,attr,omitempty"     es:"artist_id"`
	Dur     uint32   `xml:"dur,attr,omitempty"       es:"duration"`
	MID     string   `xml:"mID,attr,omitempty"       es:"music_db_id"`
	Disp    uint32   `xml:"disp,attr"                es:"display"`
	Xtitl   string   `xml:"xtitl,attr,omitempty"     es:"highband_title"`
	Xart    string   `xml:"xart,attr,omitempty"      es:"highband_artist"`
	Xpid    string   `xml:"xpid,attr,omitempty"      es:"highband_pid"`
	Rid     string   `xml:"rid,attr,omitempty"       es:"reconciliation_id"`
	Scor    string   `xml:"scor,attr,omitempty"`
	Bt      uint32   `xml:"bt,attr,omitempty"`
	Bdur    string   `xml:"bdur,attr,omitempty"`
	Bpn     string   `xml:"bpn,attr,omitempty"`
	Bct     uint32   `xml:"bct,attr,omitempty"`
	Bi      uint32   `xml:"bi,attr,omitempty"`
	Actn    string   `xml:"actn,attr,omitempty"`
	Actp    string   `xml:"actp,attr,omitempty"`
	Pldur   uint32   `xml:"pldur,attr,omitempty"     es:"playout_duration"`
	Sgdur   uint32   `xml:"sgdur,attr,omitempty"     es:"segue_duration"`
	Eph     string   `xml:"eph,attr,omitempty"`
	EID     string   `xml:"eID,attr,omitempty"`
	Soff    uint32   `xml:"soff,attr,omitempty"`
	Stitl   string   `xml:"stitl,attr,omitempty"`
	Sart    string   `xml:"sart,attr,omitempty"`
	Wtitl   string   `xml:"wtitl,attr,omitempty"`
	Wart    string   `xml:"wart,attr,omitempty"`
	Apid    string   `xml:"apid,attr,omitempty"`
	ScID    string   `xml:"scID,attr,omitempty"`
	Hmod    string   `xml:"hmod,attr,omitempty"`
	Isrc    string   `xml:"isrc,attr,omitempty"`
	Upc     string   `xml:"upc,attr,omitempty"`
}

// RootType ...
type RootType struct {
	XMLName   xml.Name       `xml:"mdsUpdate"`
	F         uint32         `xml:"f,attr"             es:"format"`
	V         string         `xml:"v,attr,omitempty"   es:"version"`
	Ev        []MdseventType `xml:"ev"`
	Heartbeat string         `xml:"heartbeat"`
}

// MdsUpdate ...
type MdsUpdate *RootType

// helper function that generates a map of the struct tags for "es"
func parseStructTags(s any) (map[string]string, error) {
	typ := reflect.TypeOf(s)

	if typ.Kind() != reflect.Struct {
		return nil, fmt.Errorf("%s is not a struct", typ)
	}

	m := make(map[string]string)

	for i := 0; i < typ.NumField(); i++ {
		fld := typ.Field(i)
		if esName := fld.Tag.Get("es"); esName != "" {
			m[fld.Name] = esName
		}
	}

	return m, nil
}
