geostore
========

An abstraction over Google's Cloud Datastore API that enables efficient geo-spatial queries.

 The geostore package implements a Store structure and Locatable interface that enable
 you to store geospatially indexed objects in Google's App Engine Datastore.
 By default (in mid 2014), Google has yet to implement a geospatial storage and search feature for Datastore
 entities.  The geostore package therefore implements a simple scheme to tag your objects with geohashes, and
 retrieve stored objects, using those geohash tags, based on location.

 The basic storage scheme is inspired by the geohash method used by the Python Geomodel project for App Engine:
 https://code.google.com/p/geomodel/
 However Geostore is not a straight Go port of Geomodel. The implementation is different from the ground up
 and as such, is not as robust, and most likely has corner case problems.
 We exploit Datastore's efficient string list indexing and lookup as described in this talk by Brett Slatkin:
 https://www.youtube.com/watch?v=AgaL6NGpkB8&list=PL15849162B82ABA20

 The basic scheme works as follows.  The map of the world is recursively divided into 4 by 4 grids of 16 cells each.
 For example, the top most grid (level 0) would look like this:

 						----------------.Lat:90, Lng:180
						| C | D | E | F |
						-----------------
						| 8 | 9 | A | B |
						-----------------
						| 4 | 5 | 6 | 7 |
						-----------------
						| 0 | 1 | 2 | 3 |
					    .----------------
		Lat:-90, Lng:-180

 Each of the grid squares in the grid above is then divided into 16 cells, and each of those into 16 more, down to
 ten levels.  The full geocode for a lat/lng point in this scheme is a string that is 10 characters long, consisting of
 symbols from 0-9 + A-F.
 The full geocode for an object at location Lat:37.781, Lng:-122.4113 in this scheme, is the string 8E64BF8FAB, however, the
 entity would be stored in the datastore with GeoBoxCodes consisting of every prefix substring, i.e. 8,8E, 8E6, ..., 8E64BF8FAB.


