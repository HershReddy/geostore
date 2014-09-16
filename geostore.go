// The geostore package implements a Store structure and Locatable interface that enable
// you to store geospatially indexed objects in Google's App Engine Datastore.
// By default (in mid 2014), Google has yet to implement a geospatial storage and search feature for Datastore
// entities.  The geostore package therefore implements a simple scheme to tag your objects with geohashes, and
// retrieve stored objects, using those geohash tags, based on location.

// The basic storage scheme is inspired by the geohash method used by the Python Geomodel project for App Engine:
// https://code.google.com/p/geomodel/
// We exploit Datastore's efficient string list indexing and lookup as described in this talk by Brett Slatkin:
// https://www.youtube.com/watch?v=AgaL6NGpkB8&list=PL15849162B82ABA20
//
// The basic scheme works as follows.  The map of the world is recursively divided into 4 by 4 grids of 16 cells each.
// For example, the top most grid (level 0) would look like this:
//
// 						----------------.Lat:90, Lng:180
//						| C | D | E | F |
//						-----------------
//						| 8 | 9 | A | B |
//						-----------------
//						| 4 | 5 | 6 | 7 |
//						-----------------
//						| 0 | 1 | 2 | 3 |
//					    .----------------
//		Lat:-90, Lng:-180
//
// Each of the grid squares in the grid above is then divided into 16 cells, and each of those into 16 more, down to
// ten levels.  The full geocode for a lat/lng point in this scheme is a string that is 10 characters long, consisting of
// symbols from 0-9 + A-F.
// The full geocode for an object at location Lat:37.781, Lng:-122.4113 in this scheme, is the string 8E64BF8FAB, however, the
// entity would be stored in the datastore with GeoBoxCodes consisting of every prefix substring, i.e. 8,8E, 8E6, ..., 8E64BF8FAB.
//
package geostore

import (
	"appengine"
	"appengine/datastore"
	"fmt"
	"log"
	"math"
	"strconv"
)

const (
	MAXLAT   float64 = 90.0
	MINLAT   float64 = -90.0
	MAXLNG   float64 = 180.0
	MINLNG   float64 = -180.0
	MAXDEPTH int     = 10
)

var CODES [4][4]string = [4][4]string{
	{"0", "1", "2", "3"},
	{"4", "5", "6", "7"},
	{"8", "9", "A", "B"},
	{"C", "D", "E", "F"},
}

type LatLng struct {
	Lat float64 `datastore:",noindex"`
	Lng float64 `datastore:",noindex"`
}

// An array that contains LatLng entities that represent the size
// of the bounding boxes at each depth level of the grid.
var BOXSIZES [MAXDEPTH + 1]LatLng

func init() {
	for i := range BOXSIZES {
		if i == 0 {
			BOXSIZES[i] = LatLng{Lat: MAXLAT - MINLAT, Lng: MAXLNG - MINLNG}
		} else {
			BOXSIZES[i] = LatLng{Lat: BOXSIZES[i-1].Lat / 4.0, Lng: BOXSIZES[i-1].Lng / 4.0}
		}
		// log.Printf("BOXSIZE for level %v is: %v", i, BOXSIZES[i])
	}
}

type LatLngBounds struct {
	NE LatLng `datastore:",noindex"`
	SW LatLng `datastore:",noindex"`
}

type GeoBoxTag string

type Locatable interface {
	SetLocation(LatLng)
	GetLocation() LatLng
	GetGeoBoxTags() []GeoBoxTag
	AddGeoBoxTag(t GeoBoxTag)
	ClearGeoBoxTags()
}

type Store struct {
	Context appengine.Context
}

type Geohasher struct {
	hash  string
	Box   LatLngBounds
	Point LatLng
}

type Error struct {
	errmsg string
}

func Log4(f float64) float64 {
	return math.Log2(f) / 2.0
}

func (e Error) Error() string {
	return fmt.Sprintf("geostore error: %s", e.errmsg)
}

func (g *Geohasher) GetHash() string {
	return g.hash
}

func (g *Geohasher) GetDepth() int {
	return len(g.hash)
}

func (p LatLng) Within(b LatLngBounds) bool {
	if p.Lat < b.SW.Lat || p.Lng < b.SW.Lng || p.Lat > b.NE.Lat || p.Lng > b.NE.Lng {
		return false
	} else {
		return true
	}
}

func (g *Geohasher) Descend() error {

	// if the Point is not within the Box we have a problem
	// something must have messed up in an earlier iteration of Descend
	if !g.Point.Within(g.Box) {
		e := Error{errmsg: "Error in Geohasher.Descend(). Point is outside Box."}
		// log.Printf("Point %v is outside Box %v", g.Point, g.Box)
		return e
	}

	latSpacing := math.Abs(g.Box.NE.Lat-g.Box.SW.Lat) / 4.0
	lngSpacing := math.Abs(g.Box.NE.Lng-g.Box.SW.Lng) / 4.0

	// log.Printf("latSpacing is: %v, and lngSpacing is: %v", latSpacing, lngSpacing)

	// if we have iterated through all the sub boxes of the current level, and the code is still X that means
	// we have a problem because the Point was not in any of the sub boxes. X is not a valid code.
	var code = "X"
	for i := 0; i < 4; i++ {
		for j := 0; j < 4; j++ {
			sub_box := LatLngBounds{
				SW: LatLng{
					Lng: g.Box.SW.Lng + (float64(j) * lngSpacing),
					Lat: g.Box.SW.Lat + (float64(i) * latSpacing),
				},
				NE: LatLng{
					Lng: g.Box.SW.Lng + (float64(j+1) * lngSpacing),
					Lat: g.Box.SW.Lat + (float64(i+1) * latSpacing),
				},
			}
			// log.Printf("sub_box is: %v, and code is: %v", sub_box, CODES[i][j])
			if g.Point.Within(sub_box) {
				code = CODES[i][j]
				g.hash = g.hash + code
				g.Box = sub_box
				// log.Printf("Geohash code computed for %v is: %v", g.Point, code)
				// log.Printf("Geohasher is: %v", g)
				break
			}
		}
		if code != "X" {
			break
		}
	}

	if code == "X" {
		e := Error{errmsg: "Could not find matching sub_box for Point in Box"}
		// log.Printf("Could not find matching sub_box for Point %v in Box %v", g.Point, g.Box)
		return e
	}

	return nil
}

func (s Store) GetEntitiesByRegion(viewbounds LatLngBounds, entityKind string, dst interface{}) ([]*datastore.Key, error) {

	geoboxtags, err := GeoBoxTagsFromViewBounds(viewbounds)
	if err != nil {
		return nil, err
	}

	keys := []*datastore.Key{}
	for _, geoboxtag := range geoboxtags {
		ks, err := datastore.NewQuery(entityKind).Filter("GeoBoxTags =", geoboxtag).GetAll(s.Context, dst)
		if err != nil {
			return nil, err
		}
		keys = append(keys, ks...)
	}

	return keys, nil
}

func (s Store) GetAllEntities(entityKind string, dst interface{}, limit int) ([]*datastore.Key, error) {
	// log.Printf("getting all entities of type %v", entityKind)
	keys, err := datastore.NewQuery(entityKind).Limit(limit).GetAll(s.Context, dst)
	return keys, err
}

func (s Store) StoreEntity(entityKind string, entity Locatable) error {

	err := GenerateGeoBoxTags(entity)
	if err != nil {
		return err
	}

	// log.Printf("Storing entity %v: %v", entityKind, entity)

	_, err = datastore.Put(s.Context, datastore.NewIncompleteKey(s.Context, entityKind, nil), entity)
	if err != nil {
		return err
	}

	return nil
}

func GenerateGeoBoxTags(l Locatable) error {
	g := &Geohasher{
		Point: l.GetLocation(),
		Box: LatLngBounds{
			NE: LatLng{Lat: MAXLAT, Lng: MAXLNG},
			SW: LatLng{Lat: MINLAT, Lng: MINLNG},
		},
		hash: "",
	}

	// log.Printf("Geohasher in GenerateGeoBoxTags() is: %v", g)
	l.ClearGeoBoxTags()

	// We only want to generate ten levels of bounding boxes, i.e GeoBoxTags with tags of at most ten characters,
	// e.g. a Locatable could have the tags:
	// 0, 0A, 0AE, 0AEF, 0AEF2, 0AEF23, 0AEF237,0AEF2378, 0AEF23789, 0AEF237898, 0AEF237898F
	// Notice how the shorter tags are always strict substrings of the longer tags. This may seem redundant
	// but it is necessary because of how string list queries work with the App Engine datastore. An entity tagged
	// with "ABCD" will not be responsive to a query for "ABC" and vice versa.

	for g.GetDepth() < MAXDEPTH {
		err := g.Descend()
		if err != nil {
			return err
		}
		l.AddGeoBoxTag(GeoBoxTag(g.hash))
	}

	return nil
}

// Finds the GeoBoxes (i.e. cells) that are the closest approximation to the given LatLngBounds
func GeoBoxTagsFromViewBounds(viewbounds LatLngBounds) ([]GeoBoxTag, error) {
	swhasher := &Geohasher{
		Point: viewbounds.SW,
		Box: LatLngBounds{
			NE: LatLng{Lat: MAXLAT, Lng: MAXLNG},
			SW: LatLng{Lat: MINLAT, Lng: MINLNG},
		},
		hash: "",
	}
	nehasher := &Geohasher{
		Point: viewbounds.NE,
		Box: LatLngBounds{
			NE: LatLng{Lat: MAXLAT, Lng: MAXLNG},
			SW: LatLng{Lat: MINLAT, Lng: MINLNG},
		},
		hash: "",
	}

	// We'll keep descending to lower levels in the grid so long as the viewbounds falls strictly within a
	// single geobox at the current level.
	for swhasher.GetHash() == nehasher.GetHash() && swhasher.GetDepth() < MAXDEPTH {
		err := swhasher.Descend()
		if err != nil {
			return nil, err
		}
		err = nehasher.Descend()
		if err != nil {
			return nil, err
		}
	}

	swhash := swhasher.GetHash()
	nehash := nehasher.GetHash()
	// This case will occur only if the region being viewed is strictly within a singe geobox (cell) at the smallest depth level of
	// of the grid.  In this case we return the tag for that smallest cell to the caller, as the entire view is in that cell
	if swhash == nehash {
		return []GeoBoxTag{GeoBoxTag(swhash)}, nil
	}

	// In all other cases the hash of the NE corner and SW corner of the viewbounds will be in different geoboxes.  The hashes will be the
	// same except for the last character in each hash.  Those last characters determine where the NE/SW corners of the view bounds fall
	// at the current depth level. We determine the slice of GeoBoxTags to return by figuring out all the geoboxes that
	// are between the geobox tag of the SW corner and the geobox tag of the NE corner.

	hashlen := len(swhash)
	if hashlen != len(nehash) {
		return nil, Error{errmsg: "geostore error: FindGeoBoxBounds error: swhash length does not equal nehash length"}
	}
	prefix := swhash[0 : hashlen-1]

	swbox := swhash[hashlen-1 : hashlen]
	nebox := nehash[hashlen-1 : hashlen]

	swint, err := strconv.ParseInt(swbox, 16, 0)
	if err != nil {
		return nil, err
	}
	neint, err := strconv.ParseInt(nebox, 16, 0)
	if err != nil {
		return nil, err
	}

	// We will fill the suffixes slice with the single letter codes for the geoboxes that are within the viewbounds at the current depth
	suffixes := []string{}
	for i := swint / 4; i <= neint/4; i++ {
		for j := swint % 4; j <= neint%4; j++ {
			suffixes = append(suffixes, CODES[i][j])
		}
	}

	// Now add the prefix to get the geoboxtags we need to return
	geoboxtags := []GeoBoxTag{}
	for _, suffix := range suffixes {
		geoboxtags = append(geoboxtags, GeoBoxTag(prefix+suffix))
	}
	log.Printf("geoboxtags for %v: \n %v \n", viewbounds, geoboxtags)
	return geoboxtags, nil
}
