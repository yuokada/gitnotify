package main

import (
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"sort"

	"github.com/gorilla/mux"
	"github.com/gorilla/sessions"
	"github.com/markbates/goth"
	"github.com/markbates/goth/gothic"
	"github.com/markbates/goth/providers/github"
)

// TODO rename to Authentication
// data/$provider/$user/settings.yml
type userInfoSession struct {
	Provider string `yaml:"provider"` // github/gitlab
	Name     string `yaml:"name"`     // name of the person addressing to
	Email    string `yaml:"email"`    // email that we will send to
	UserName string `yaml:"username"` // username for identification
	Token    string `yaml:"token"`    // used to query the provider
}

type Authentication userInfoSession

func (userInfo *userInfoSession) getConfigFile() string {
	if userInfo.Provider == "" {
		return ""
	}
	return fmt.Sprintf("data/%s/%s/settings.yml", userInfo.Provider, userInfo.UserName)
}

// ProviderIndex is used for setting up the providers
type ProviderIndex struct {
	Providers    []string
	ProvidersMap map[string]string
}

func init() {
	gothic.Store = sessions.NewFilesystemStore(os.TempDir(), []byte("goth-example"))
	gothic.GetProviderName = getProviderName
}

// load envconfig via https://github.com/kelseyhightower/envconfig
func initAuth(p *mux.Router) {
	goth.UseProviders(
		github.New(os.Getenv("GITHUB_KEY"), os.Getenv("GITHUB_SECRET"), serverProto+host+"/auth/github/callback"),
	)

	m := make(map[string]string)
	m["github"] = "Github"

	var keys []string
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	providerIndex := &ProviderIndex{Providers: keys, ProvidersMap: m}

	// p := pat.New()
	p.HandleFunc("/{provider}/callback", func(res http.ResponseWriter, req *http.Request) {

		user, err := gothic.CompleteUserAuth(res, req)
		if err != nil {
			fmt.Fprintln(res, err)
			return
		}
		authType, _ := getProviderName(req)
		userInfo := &userInfoSession{
			Provider: authType,
			UserName: user.NickName,
			Token:    user.AccessToken,
		}
		hc := &httpContext{res, req}
		hc.setSession(userInfo)
		http.Redirect(res, req, homePageForLoggedIn, 302)
	}).Methods("GET")

	p.HandleFunc("/{provider}", func(res http.ResponseWriter, req *http.Request) {
		hc := &httpContext{res, req}
		if hc.isUserLoggedIn() {
			text := "User is already logged in"
			displayText(hc, res, text)
		} else {
			gothic.BeginAuthHandler(res, req)
		}
	}).Methods("GET")

	p.HandleFunc("/", func(res http.ResponseWriter, req *http.Request) {
		t, _ := template.New("foo").Parse(indexTemplate)
		t.Execute(res, providerIndex)
	}).Methods("GET")
}

// See gothic/gothic.go: GetProviderName function
// Overridden since we use mux
func getProviderName(req *http.Request) (string, error) {
	vars := mux.Vars(req)
	provider := vars["provider"]
	if provider == "" {
		return provider, errors.New("you must select a provider")
	}
	return provider, nil
}

var indexTemplate = `{{range $key,$value:=.Providers}}
    <p><a href="/auth/{{$value}}">Log in with {{index $.ProvidersMap $value}}</a></p>
{{end}}`

var userTemplate = `
<p>Name: {{.Name}} [{{.LastName}}, {{.FirstName}}]</p>
<p>Email: {{.Email}}</p>
<p>NickName: {{.NickName}}</p>
<p>Location: {{.Location}}</p>
<p>AvatarURL: {{.AvatarURL}} <img src="{{.AvatarURL}}"></p>
<p>Description: {{.Description}}</p>
<p>UserID: {{.UserID}}</p>
<p>AccessToken: {{.AccessToken}}</p>
<p>ExpiresAt: {{.ExpiresAt}}</p>
<p>RefreshToken: {{.RefreshToken}}</p>
`
