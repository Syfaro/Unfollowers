package main

import (
	"database/sql"
	"encoding/json"
	"github.com/ChimeraCoder/anaconda"
	"github.com/garyburd/go-oauth/oauth"
	"log"
	"net/http"
	"strconv"
)

var tempCredentials *oauth.Credentials
var listenHost string

func authTwitter(w http.ResponseWriter, r *http.Request) {
	authURL, tempCred, err := anaconda.AuthorizationURL(
		"http://" + listenHost + "/auth/callback")
	if err != nil {
		log.Fatal(err)
	}

	tempCredentials = tempCred

	w.Header().Set("Location", authURL)

	w.WriteHeader(http.StatusFound)

}

func authTwitterCallback(w http.ResponseWriter, r *http.Request) {
	verifier := r.URL.Query().Get("oauth_verifier")

	creds, _, err := anaconda.GetCredentials(tempCredentials,
		verifier)

	api := anaconda.NewTwitterApi(creds.Token, creds.Secret)

	self, err := api.GetSelf(nil)
	if err != nil {
		log.Fatal(err)
	}

	var u user
	err = db.Get(&u, `select id from tokens where twitter_id = ?`,
		self.Id)
	if err == sql.ErrNoRows {
		_, err = db.Exec(`insert into tokens
					(twitter_id, token, secret, screen_name,
						display_name) values
				(?, ?, ?, ?, ?)`, self.Id, creds.Token, creds.Secret,
			self.ScreenName, self.Name)
		if err != nil {
			log.Fatal(err)
		}
	} else {
		db.Exec(`update tokens set
						screen_name = ?, display_name = ?
					where twitter_id = ?`, self.ScreenName, self.Name,
			self.Id)
	}

	w.Header().Set("Location", "http://"+listenHost)

	w.WriteHeader(http.StatusFound)

}

func accountSelect(w http.ResponseWriter, r *http.Request) {
	w.Write(MustAsset("data/account_select.html"))
}

func accountView(w http.ResponseWriter, r *http.Request) {
	w.Write(MustAsset("data/index.html"))
}

func accountLoad(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")

	tokenID, _ := strconv.ParseInt(token, 10, 64)

	w.Header().Set("Content-type", "text/event-stream")

	load(tokenID, w)
}

func tokensList(w http.ResponseWriter, r *http.Request) {
	var tokens []token
	db.Select(&tokens, `select * from tokens`)

	j, _ := json.Marshal(tokens)

	w.Write(j)
}

func configFetch(w http.ResponseWriter, r *http.Request) {
	var cfg []config
	db.Select(&cfg, `select * from config`)

	configMap := make(map[string]string)

	for _, item := range cfg {
		configMap[item.Key] = item.Value
	}

	j, _ := json.Marshal(configMap)

	w.Write(j)
}

func configUpdate(w http.ResponseWriter, r *http.Request) {
	k, v := r.URL.Query().Get("key"), r.URL.Query().Get("value")

	var cfg config
	err := db.Get(&cfg, `select * from config where key = ?`, k)
	if err == sql.ErrNoRows {
		res, _ := db.Exec(`insert into config (key, value) values (?, ?)`, k, v)
		lastID, _ := res.LastInsertId()

		db.Get(&cfg, `select * from config where id = ?`, lastID)
	} else {
		if v == "_delete" {
			db.Exec(`delete from config where key = ?`, k)
		} else {
			db.Exec(`update config set value = ? where key = ?`, v, k)
		}
	}

	db.Get(&cfg, `select * from config where key = ?`, k)
	j, _ := json.Marshal(cfg)

	w.Write(j)
}

func assetFetch(w http.ResponseWriter, r *http.Request) {
	t := r.URL.Query().Get("type")
	a, err := Asset("data/" + r.URL.Query().Get("name") + "." + t)
	if err != nil {
		w.Write([]byte("Unable to find asset."))
		w.WriteHeader(http.StatusNotFound)

		return
	}

	switch t {
	case "js":
		w.Header().Set("Content-type", "application/javascript")
	case "css":
		w.Header().Set("Content-type", "text/css")
	}

	w.Write(a)
}

func followersKnown(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")

	var users []user

	db.Select(&users, `select users.* from events t1
				join (select user_id, max(event_date) event_date from
					events where token_id = ?
						group by token_id, user_id) t2
						on t1.user_id = t2.user_id
				inner join users on t1.user_id = users.id
				where event_type = 'f' group by users.id`, token)

	j, _ := json.Marshal(users)

	w.Write(j)
}

func followersLatest(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")

	tokenID, _ := strconv.ParseInt(token, 10, 64)

	var unfollowers []userEvent
	err := db.Select(&unfollowers, `select users.*, t2.event_date from events t1
				join (select user_id, max(event_date) event_date from
					events where token_id = ? and event_type = 'u'
						group by token_id, user_id) t2
						on t1.user_id = t2.user_id
				inner join users on t1.user_id = users.id
				where t1.event_type = 'u'
				group by users.id order by t2.event_date desc limit 5`, tokenID)
	if err != nil {
		log.Println(err)
	}

	var followers []userEvent
	err = db.Select(&followers, `select users.*, t2.event_date from events t1
				join (select user_id, max(event_date) event_date from
					events where token_id = ? and event_type = 'f'
						group by token_id, user_id) t2
						on t1.user_id = t2.user_id
				inner join users on t1.user_id = users.id
				where t1.event_type = 'f'
				group by users.id order by t2.event_date desc limit 5`, tokenID)
	if err != nil {
		log.Println(err)
	}

	data := struct {
		Followers   []userEvent `json:"followers"`
		Unfollowers []userEvent `json:"unfollowers"`
	}{
		Followers:   followers,
		Unfollowers: unfollowers,
	}

	j, _ := json.Marshal(data)

	w.Write(j)
}

func getAllEvents(tokenID int64, event string) []userEvent {
	var unfollowers []userEvent
	err := db.Select(&unfollowers, `select users.*, t2.event_date from events t1
				join (select user_id, max(event_date) event_date from
					events where token_id = ? and event_type = ?
						group by token_id, user_id) t2
						on t1.user_id = t2.user_id
				inner join users on t1.user_id = users.id
				where t1.event_type = ?
				group by users.id order by t2.event_date desc`, tokenID, event, event)
	if err != nil {
		log.Println(err)
	}

	return unfollowers
}

func followersAll(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	event := r.URL.Query().Get("event")

	tokenID, _ := strconv.ParseInt(token, 10, 64)

	var data interface{}

	if event == "" {
		data = struct {
			Followers   []userEvent `json:"followers"`
			Unfollowers []userEvent `json:"unfollowers"`
		}{
			Followers:   getAllEvents(tokenID, "f"),
			Unfollowers: getAllEvents(tokenID, "u"),
		}
	} else {
		data = getAllEvents(tokenID, event)
	}

	j, _ := json.MarshalIndent(data, "", "  ")

	w.Write(j)
}
