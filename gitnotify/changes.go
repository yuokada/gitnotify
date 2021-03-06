package gitnotify

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/sairam/kinli"
)

// Generic structure which should fit any notification to be sent the user
type gnDiffDatum []*gnDiffData

type gnDiffData struct {
	Repo    link       `json:"repo"`
	Changed bool       `json:"changed"`
	Data    []diffData `json:"data"`
	MadeFor string     `json:"made_for"`
}

type diffData struct {
	Title      link   `json:"title"`
	Error      string `json:"error"`
	ChangeType string `json:"change_type"`
	Changed    bool   `json:"changed"`
	Changes    []link `json:"changes"`
}

type link struct {
	Text  string `json:"text"`
	Href  string `json:"href"`
	Title string `json:"title"`
}

func listAllDiffs(w http.ResponseWriter, r *http.Request) {

	hc := &kinli.HttpContext{W: w, R: r}
	// Redirect user if not logged in
	if hc.RedirectUnlessAuthed(loginFlash) {
		return
	}

	userInfo := getUserInfo(hc)
	configFile := userInfo.getConfigFile()

	conf := new(Setting)
	conf.load(configFile)
	files := (&gnDiffDatum{}).ListUserChanges(conf)

	page := kinli.NewPage(hc, "Changes for "+conf.Auth.UserName, userInfo, files, nil) // "Recent Changes"
	kinli.DisplayPage(w, "changes_list", page)

	// TODO render in Page
}

func renderThisDiff(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	entry := vars["diffentry"]
	if entry == "" {
		listAllDiffs(w, r)
		return
	}

	hc := &kinli.HttpContext{W: w, R: r}
	// Redirect user if not logged in
	if hc.RedirectUnlessAuthed(loginFlash) {
		return
	}

	userInfo := getUserInfo(hc)
	configFile := userInfo.getConfigFile()

	conf := new(Setting)
	conf.load(configFile)

	diffs := &gnDiffDatum{}
	if err := diffs.load(entry, conf); err != nil {
		http.NotFound(w, r)
		return
	}

	intFilename, _ := strconv.ParseInt(entry, 10, 64)
	reference := parseUnixTimeToString(intFilename, "02 Jan 2006 | 15 Hrs", conf.User.TimeZoneName)

	title := "New Updates from " + reference
	log.Println("TODO: ", title)
	page := kinli.NewPage(hc, "Changes for "+entry, userInfo, diffs, nil)
	kinli.DisplayPage(w, "changes", page)
}

// check if atleast one of the diffs has changed
func (r *gnDiffDatum) hasChanges() bool {
	// check if eligible to send email
	for _, a := range *r {
		if a.Changed {
			return true
		}
	}
	return false
}

type changeDetail struct {
	Display   string
	Reference int64
}

// ByInt ..
type ByInt []*changeDetail

func (a ByInt) Len() int           { return len(a) }
func (a ByInt) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByInt) Less(i, j int) bool { return a[i].Reference < a[j].Reference }

// at a user/repo level
func (r *gnDiffDatum) ListUserChanges(conf *Setting) []*changeDetail {
	dir := strings.Join([]string{conf.Auth.getConfigDir(), "diff"}, string(os.PathSeparator))

	fis, err := ioutil.ReadDir(dir)
	if err != nil {
		log.Print(err)
		return []*changeDetail{}
	}
	files := make([]*changeDetail, 0, len(fis))
	for _, fi := range fis {
		fileName := strings.TrimRight(fi.Name(), ".json")
		intFilename, _ := strconv.ParseInt(fileName, 10, 64)
		reference := parseUnixTimeToString(intFilename, "02 Jan 2006 | 15 Hrs", conf.User.TimeZoneName)
		files = append(files, &changeDetail{reference, intFilename})
	}
	sort.Sort(sort.Reverse(ByInt(files)))

	return files
}

func parseUnixTimeToString(i int64, format string, tz string) string {
	loc, _ := time.LoadLocation(tz)
	ti := time.Unix(i, 0).In(loc)

	return ti.Format(format)
}

func (r *gnDiffDatum) save(conf *Setting) (string, error) {
	var out []byte
	var err error

	t := time.Now()

	dir := strings.Join([]string{conf.Auth.getConfigDir(), "diff"}, string(os.PathSeparator))
	os.MkdirAll(dir, 0700)

	filenamePrefix := fmt.Sprintf("%d", t.Unix())
	fileName := strings.Join([]string{conf.Auth.getConfigDir(), "diff", filenamePrefix + ".json"}, string(os.PathSeparator))

	if out, err = json.Marshal(r); err != nil {
		fmt.Println("Error saving diff ", err)
		return "", err
	}

	if err = saveCompressedFile(fileName, out); err != nil {
		return "", err
	}

	return filenamePrefix, nil
}

func (r *gnDiffDatum) load(fileNamePrefix string, conf *Setting) error {
	fileName := strings.Join([]string{conf.Auth.getConfigDir(), "diff", fileNamePrefix + ".json"}, string(os.PathSeparator))
	data, err := readCompressedFile(fileName)
	if err != nil {
		return err
	}
	json.Unmarshal(data, &r)
	return nil
}

func saveCompressedFile(fileName string, data []byte) error {
	file, err := os.OpenFile(fileName, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer file.Close()

	fileWriter := gzip.NewWriter(file)
	fileWriter.Write(data)
	fileWriter.Close()

	return nil
}

func readCompressedFile(fileName string) ([]byte, error) {
	file, err := os.OpenFile(fileName, os.O_RDONLY, 0600)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	fileReader, err := gzip.NewReader(file)
	if err != nil {
		return nil, err
	}
	defer fileReader.Close()

	data, err := ioutil.ReadAll(fileReader)
	if err != nil {
		return nil, err
	}

	return data, nil
}
