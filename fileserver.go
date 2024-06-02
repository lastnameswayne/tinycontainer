package main

// file
// open tar and content address all the files

//this one gets called when runc wants to read a file and its not cached
//on the local worker (the worker is in the cloud also)
//so on startup we build an index
//should laos be content addressed
//build images using docker
//docker save to get a tar file
//then we can checksum every file ()
//this one has an index of the same form as the worker

// Upload depends on if the worker reads the dockerfile on startup and then sends everything
// over here. Or if tje
// Fetch is always needed
type Server interface {
	Upload(filecontent []byte)
	Fetch(filehash string) []byte
}

// should expose endpoints for the methods below
// explre nginx
type server struct{}

// given a file, hash it and store it in a map, write to SSD in a content-addressed way
// can use the bitcask for this
func (s *server) Upload() {}

// looks up in the index to get the hash (which is the location) and returns the file if there
func (s *server) Fetch() {}
