package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"log"
	"math/rand"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Artist struct {
	Name   string
	Albums map[string]*Album
}

type Album struct {
	Name  string
	Songs []*Song
}

type Song struct {
	Artist   string
	Album    string
	Title    string
	Filename string
	Filesize int
}

type WeightedSong struct {
	Song   *Song
	Weight float64
}

var artistLookup = make(map[string]*Artist)

func getArtist(name string) *Artist {
	var a = artistLookup[name]
	if a == nil {
		a = &Artist{}
		artistLookup[name] = a
		a.Albums = make(map[string]*Album)
	}
	return a
}

func (artist *Artist) getAlbum(name string) *Album {
	var al = artist.Albums[name]
	if al == nil {
		al = &Album{}
		artist.Albums[name] = al
		al.Name = name
		al.Songs = make([]*Song, 0)
	}
	return al
}

func (artist *Artist) addSong(song *Song) {
	a := artist.getAlbum(song.Album)
	a.Songs = append(a.Songs, song)
}

func (artist *Artist) getAlbums() []*Album {
	var albums []*Album
	for k, _ := range artist.Albums {
		album := artist.Albums[k]
		albums = append(albums, album)
	}
	return albums
}

func listAll() {
	var artists []string
	for k := range artistLookup {
		artists = append(artists, k)
	}
	sort.Strings(artists)
	for _, k := range artists {
		artist := artistLookup[k]
		fmt.Println("Artist: ", artist.Name)
		albums := artist.getAlbums()
		for _, album := range albums {
			fmt.Println(" ", album.Name)
			for _, song := range album.Songs {
				fmt.Println("   ", song.Title)
			}
		}
	}
}

func main() {
	var srcDir = flag.String("src", "."+string(filepath.Separator), "the directory where we find our mp3s")
	var destDir = flag.String("dest", "."+string(filepath.Separator), "the directory where we should put the copies")

	flag.Parse()

	loadSongs(srcDir)
	songs := getDistributedRandom()

	fmt.Println("Should write to: ", destDir)

	for _, song := range songs {
		fmt.Println(song.Artist, " - ", song.Title)
	}
}

//getTrailingBytes opens a file and reads the last n bytes
func getTrailingBytes(filename string, n int) ([]byte, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	_, err = f.Seek(-int64(n), os.SEEK_END)
	if err != nil {
		return nil, err
	}
	b := make([]byte, n)
	_, err = f.Read(b)
	if err != nil {
		return nil, err
	}
	return b, nil
}

//Tag represents anything that can produce a list of details
type Tag interface {
	//Parse returns the complete list of all data found in the tag
	Parse() map[string]interface{}
	//String returns the canonical formatted string
	String() string
}

//mp3ID3v1 is a specific kind of tagging
type mp3ID3v1 []byte

//Parse decodes the ID3v1 tag
//According to wikipedia, track number is in here somewhere too
//http://en.wikipedia.org/wiki/ID3#Layout
func (mp3 mp3ID3v1) Parse() map[string]interface{} {
	m := make(map[string]interface{}, 8)
	if string(mp3[:3]) != "TAG" {
		return nil
	}
	m["title"] = strings.TrimSpace(string(mp3[3:33]))
	m["artist"] = strings.TrimSpace(string(mp3[33:63]))
	m["album"] = strings.TrimSpace(string(mp3[63:93]))
	m["year"] = strings.TrimSpace(string(mp3[93:97]))
	m["comment"] = strings.TrimSpace(string(mp3[97:126]))
	m["genre"] = int(mp3[127])
	return m
}

//If a particular Tag had additional fields (personal rating?)
//we could provide a different function to display them
func (mp3 mp3ID3v1) String() string {
	return defaultFormat(mp3.Parse())
}

func keyEqualsValue(m map[string]interface{}, s string) string {
	return fmt.Sprintf("%s=%v\n", s, m[s])
}

//defaultFormat should display the tag information like the example
func defaultFormat(m map[string]interface{}) (s string) {
	s += keyEqualsValue(m, "album")
	s += keyEqualsValue(m, "artist")
	s += keyEqualsValue(m, "title")
	s += keyEqualsValue(m, "genre")
	s += keyEqualsValue(m, "year")
	s += keyEqualsValue(m, "comment")
	return
}

func loadSongs(srcDir *string) {
	files, _ := filepath.Glob(path.Join(*srcDir, "*.mp3"))

	for _, f := range files {
		b, err := getTrailingBytes(f, 128)
		if err != nil {
			log.Fatal(err)
		}
		var tag = mp3ID3v1(b)
		var fields = tag.Parse()

		song := &Song{}
		song.Title = fields["title"].(string)
		song.Album = fields["album"].(string)
		song.Artist = fields["artist"].(string)
		song.Filename = f

		fi, _ := os.Stat(f)
		song.Filesize = int(fi.Size())

		artist := getArtist(song.Artist)
		artist.addSong(song)
	}
}

type ByWeight []*WeightedSong

func (a ByWeight) Len() int           { return len(a) }
func (a ByWeight) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByWeight) Less(i, j int) bool { return a[i].Weight < a[j].Weight }

func getDistributedRandom() []*Song {
	var allSongs = make([]*WeightedSong, 0)
	rand.Seed(time.Now().UnixNano())

	for k := range artistLookup {
		artist := artistLookup[k]
		albums := artist.getAlbums()

		weightedSongs := make([]*WeightedSong, 0)
		for _, album := range albums {
			songIndicies := rand.Perm(len(album.Songs))

			distribution := float64(1) / float64(len(album.Songs)+1)
			variability := distribution / float64(2)
			distribution -= variability
			variability *= 2

			current := float64(0)

			for idx := range songIndicies {
				song := album.Songs[idx]
				weighted := &WeightedSong{}
				weighted.Song = song

				current += distribution
				current += rand.Float64() * variability
				weighted.Weight = current
				weightedSongs = append(weightedSongs, weighted)
			}
		}

		sort.Sort(ByWeight(weightedSongs))

		distribution := float64(1) / float64(len(weightedSongs)+1)
		variability := distribution / float64(2)
		distribution -= variability
		variability *= 2

		current := float64(0)

		for _, s := range weightedSongs {
			current += distribution
			current += rand.Float64() * variability
			s.Weight = current

			allSongs = append(allSongs, s)
		}
	}

	sort.Sort(ByWeight(allSongs))
	songs := make([]*Song, len(allSongs))
	for i := 0; i < len(allSongs); i++ {
		songs[i] = allSongs[i].Song
	}

	return songs
}

func basicRandom(srcDir *string, destDir *string) {
	files, _ := filepath.Glob(path.Join(*srcDir, "*.mp3"))

	rand.Seed(time.Now().UnixNano())
	fileOrder := rand.Perm(len(files))

	idx := 1
	currentFolder := 0
	var maxFolderSize int64 = 629145600 // 600 MB. My Cd player is crappy :-(
	currentFolderSize := maxFolderSize + 1
	newIdx := 1

	currentFolderPath := ""

	for _, f := range fileOrder {
		var buffer bytes.Buffer
		fName := files[f]

		title := fName[strings.LastIndex(fName, string(filepath.Separator))+1:]

		buffer.WriteString(fmt.Sprintf("%03d", newIdx))
		buffer.WriteString(" - ")
		cFound := false
		spFound := false
		for _, c := range title {
			if cFound {
				buffer.WriteRune(c)
				continue
			}

			// skip leading numbers, dashes and spaces
			if !spFound && (c >= '0' && c <= '9') {
				continue
			}

			if !spFound && (c == ' ') {
				spFound = true
			}

			if !cFound && (c == ' ' || c == '-') {
				continue
			}
			cFound = true
			buffer.WriteRune(c)
		}

		newFilename := buffer.String()
		idx++
		newIdx++

		fmt.Printf("%s\n", newFilename)

		if currentFolderSize > maxFolderSize {
			currentFolder++
			currentFolderSize = 0
			currentFolderPath = path.Join(*destDir, fmt.Sprintf("%02d", currentFolder))
			os.Mkdir(currentFolderPath, 0666)
			newIdx = 1
		}

		fi, _ := os.Stat(fName)
		currentFolderSize += fi.Size()

		destName := path.Join(currentFolderPath, newFilename)
		cp(destName, fName)
		//cpCmd := exec.Command("cp", "", strings.Replace(fName, " ", "\\", -1), strings.Replace(destName, " ", "\\", -1))
		//err := cpCmd.Run()
		//if err != nil {
		//	fmt.Println(err)
		//}
	}
}

func cp(dst, src string) error {
	s, err := os.Open(src)
	if err != nil {
		return err
	}
	// no need to check errors on read only file, we already got everything
	// we need from the filesystem, so nothing can go wrong now.
	defer s.Close()
	d, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(d, s); err != nil {
		d.Close()
		return err
	}
	return d.Close()
}
