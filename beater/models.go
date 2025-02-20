package beater

import (
	"encoding/xml"
	"fmt"
	"log"
	"reflect"
)

var EventFieldMap = map[string]string{}

// save the struct "es" tags
func init() {
	var err error
	if EventFieldMap, err = parseStructTags(MdseventType{}); err != nil {
		log.Fatal(err)
	}
}

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
	Scor    string   `xml:"scor,attr,omitempty"      es:"sports_score"`
	Bt      uint32   `xml:"bt,attr,omitempty"        es:"parent_type_id"`
	Bdur    string   `xml:"bdur,attr,omitempty"      es:"parent_duration"`
	Bpn     string   `xml:"bpn,attr,omitempty"       es:"parent_name"`
	Bct     uint32   `xml:"bct,attr,omitempty"       es:"child_count"`
	Bi      uint32   `xml:"bi,attr,omitempty"        es:"child_index"`
	Actn    string   `xml:"actn,attr,omitempty"      es:"action"`
	Actp    string   `xml:"actp,attr,omitempty"      es:"action_param"`
	Pldur   uint32   `xml:"pldur,attr,omitempty"     es:"playout_duration"`
	Sgdur   uint32   `xml:"sgdur,attr,omitempty"     es:"segue_duration"`
	Eph     string   `xml:"eph,attr,omitempty"       es:"ephemeral"`
	EID     string   `xml:"eID,attr,omitempty"       es:"sports_event_id"`
	Soff    uint32   `xml:"soff,attr,omitempty"      es:"start_offset"`
	Stitl   string   `xml:"stitl,attr,omitempty"     es:"lowband_title"`
	Sart    string   `xml:"sart,attr,omitempty"      es:"lowband_artist"`
	Wtitl   string   `xml:"wtitl,attr,omitempty"     es:"streaming_title"`
	Wart    string   `xml:"wart,attr,omitempty"      es:"streaming_artist"`
	Apid    string   `xml:"apid,attr,omitempty"      es:"apg_sports_pids"`
	ScID    string   `xml:"scID,attr,omitempty"      es:"schedule_id"`
	Hmod    string   `xml:"hmod,attr,omitempty"      es:"hybrid_mod"`
	Isrc    string   `xml:"isrc,attr,omitempty"      es:"isrc"`
	Upc     string   `xml:"upc,attr,omitempty"       es:"upc"`
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
