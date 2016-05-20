package main

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/ChimeraCoder/anaconda"
	"github.com/garyburd/go-oauth/oauth"
	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
	"github.com/skratchdot/open-golang/open"
)

var db *sqlx.DB

type user struct {
	ID int64 `json:"db_id" db:"id"`

	TwitterID   int64  `json:"id" db:"twitter_id"`
	ScreenName  string `json:"screen_name" db:"screen_name"`
	DisplayName string `json:"name" db:"display_name"`

	ProfileIcon string `json:"profile_image_url_https" db:"profile_icon"`
	Color       string `json:"profile_link_color" db:"color"`
}

type userEvent struct {
	user
	EventDate time.Time `db:"event_date" json:"date"`
}

type token struct {
	ID int64 `json:"db_id" db:"id"`

	TwitterID   int64  `json:"id" db:"twitter_id"`
	ScreenName  string `json:"screen_name" db:"screen_name"`
	DisplayName string `json:"name" db:"display_name"`

	Token  string `json:"-" db:"token"`
	Secret string `json:"-" db:"secret"`
}

type config struct {
	ID int64 `db:"id"`

	Key   string `db:"key"`
	Value string `db:"value"`
}

type event struct {
	ID int64 `db:"id"`

	TokenID int64 `db:"token_id"`
	UserID  int64 `db:"user_id"`

	EventType string    `db:"event_type"`
	EventDate time.Time `db:"event_time"`
}

type userToCheck struct {
	User             interface{}
	IsStillFollowing bool
	IsInDatabase     bool
}

const (
	TwitterConsumer = "iR0mPmyUl4IQX4ebSZGe60UpM"
	TwitterSecret   = "rFP2xPufsKa0NUWbkVuhLoJIWnhyEfJWiJ0htGKJ1Lnkd8klyr"
)

var tempCredentials *oauth.Credentials

func startServer() {
	http.HandleFunc("/",
		func(w http.ResponseWriter, r *http.Request) {
			w.Write(MustAsset("data/account_select.html"))
		})

	http.HandleFunc("/account",
		func(w http.ResponseWriter, r *http.Request) {
			w.Write(MustAsset("data/index.html"))
		})

	http.HandleFunc("/known",
		func(w http.ResponseWriter, r *http.Request) {
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
		})

	http.HandleFunc("/latest",
		func(w http.ResponseWriter, r *http.Request) {
			token := r.URL.Query().Get("token")

			tokenID, _ := strconv.ParseInt(token, 10, 64)

			var unfollowers []userEvent
			err := db.Select(&unfollowers, `select users.*, t1.event_date from events t1
				join (select user_id, max(event_date) event_date from
					events where token_id = ?
						group by token_id, user_id) t2
						on t1.user_id = t2.user_id
				inner join users on t1.user_id = users.id
				where t1.event_type = 'u'
				group by users.id order by t1.event_date desc limit 5`, tokenID)
			if err != nil {
				log.Println(err)
			}

			var followers []userEvent
			err = db.Select(&followers, `select users.*, t1.event_date from events t1
				join (select user_id, max(event_date) event_date from
					events where token_id = ?
						group by token_id, user_id) t2
						on t1.user_id = t2.user_id
				inner join users on t1.user_id = users.id
				where t1.event_type = 'f'
				group by users.id order by t1.event_date desc limit 5`, tokenID)
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
		})

	http.HandleFunc("/tokens",
		func(w http.ResponseWriter, r *http.Request) {
			var tokens []token
			db.Select(&tokens, `select * from tokens`)

			j, _ := json.Marshal(tokens)

			w.Write(j)
		})

	http.HandleFunc("/load",
		func(w http.ResponseWriter, r *http.Request) {
			token := r.URL.Query().Get("token")

			tokenID, _ := strconv.ParseInt(token, 10, 64)

			w.Header().Set("Content-type", "text/event-stream")

			load(tokenID, w)
		})

	http.HandleFunc("/auth",
		func(w http.ResponseWriter, r *http.Request) {
			authURL, tempCred, err := anaconda.AuthorizationURL(
				"http://127.0.0.1:8080/auth/callback")
			if err != nil {
				log.Fatal(err)
			}

			tempCredentials = tempCred

			w.Header().Set("Location", authURL)

			w.WriteHeader(http.StatusFound)

		})

	http.HandleFunc("/auth/callback",
		func(w http.ResponseWriter, r *http.Request) {
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

			w.Header().Set("Location", "http://127.0.0.1:8080/")

			w.WriteHeader(http.StatusFound)

		})

	http.HandleFunc("/asset",
		func(w http.ResponseWriter, r *http.Request) {
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
		})

	http.HandleFunc("/config",
		func(w http.ResponseWriter, r *http.Request) {
			var cfg []config
			db.Select(&cfg, `select * from config`)

			j, _ := json.Marshal(cfg)

			w.Write(j)
		})

	http.HandleFunc("/config/update",
		func(w http.ResponseWriter, r *http.Request) {
			k, v := r.URL.Query().Get("key"), r.URL.Query().Get("value")

			var cfg config
			err := db.Get(&cfg, `select * from config where key = ?`, k)
			if err == sql.ErrNoRows {
				res, _ := db.Exec(`insert into config (key, value) values (?, ?)`, k, v)
				lastID, _ := res.LastInsertId()

				db.Get(&cfg, `select * from config where id = ?`, lastID)
			} else {
				db.Exec(`update config set value = ? where key = ?`, v, k)
			}
		})

	go func() {
		time.Sleep(time.Second)
		open.Run("http://127.0.0.1:8080")
	}()

	http.ListenAndServe("127.0.0.1:8080", nil)
}

func min(a, b int) int {
	if a <= b {
		return a
	}

	return b
}

func load(tokenToFetchWith int64, w http.ResponseWriter) {
	f, ok := w.(http.Flusher)
	if !ok {
		w.Write([]byte("event: error\ndata: Bad HTTP request?\n\n"))
		log.Println("Somehow HTTP request didn't have a flusher")
		return
	}

	// A token struct containg current auth tokens
	var tokenSet token

	// Fetch tokens from database for that ID
	err := db.Get(&tokenSet, `select * from tokens where id = ?`, tokenToFetchWith)

	// Something happened, can't find tokens.
	if err != nil {
		w.Write([]byte("event: error\ndata: Error loading tokens: " +
			err.Error() + "\n\n"))
		f.Flush()

		log.Printf("Error loading tokens: %s\n", err)
		return
	}

	// Twitter API instance
	api := anaconda.NewTwitterApi(tokenSet.Token, tokenSet.Secret)

	// Get current user to update DB
	self, err := api.GetSelf(nil)
	if err != nil {
		log.Fatal(err)
	}

	w.Write([]byte("event: status\ndata: Stored tokens are valid\n\n"))
	f.Flush()

	// Update database in case the user changed their profile.
	db.Exec(`update tokens
		set screen_name = ?, display_name = ? where id = ?`,
		self.ScreenName, self.Name, tokenSet.ID)

	// Collect a list of current follower IDs from Twitter.
	followers_id := api.GetFollowersIdsAll(nil)

	all_ids := []int64{}
	// Go through each page in Twitter results and collect IDs.
	for page := range followers_id {
		all_ids = append(all_ids, page.Ids...)
	}

	w.Write([]byte("event: status\ndata: Found " +
		strconv.Itoa(len(all_ids)) + " followers\n\n"))
	f.Flush()

	// All known users currently attached to this token.
	// Starts with current Twitter users, then loads from DB.
	currentUsers := make(map[int64]userToCheck)

	for i := 0; i < len(all_ids); i += 100 {
		batch := all_ids[i:min(i+100, len(all_ids))]

		lookup, err := api.GetUsersLookupByIds(batch, nil)
		if err != nil {
			log.Panic(err)
		}

		w.Write([]byte("event: status\ndata: Loaded group of users\n\n"))
		f.Flush()

		for _, follower := range lookup {
			// Okay, store that we got this user currently.
			currentUsers[follower.Id] = userToCheck{
				User:             follower,
				IsStillFollowing: true,
				IsInDatabase:     false, // Overwritten later
			}

			j, _ := json.Marshal(follower)

			w.Write([]byte("event: user\ndata: "))
			w.Write(j)
			w.Write([]byte("\n\n"))
			f.Flush()
		}
	}

	// Users currently in database attached to user.
	var storedUsers []user

	db.Select(&storedUsers, `select * from users where id in
		(select user_id from events where token_id = ?
			group by user_id)`, tokenToFetchWith)

	for _, u := range storedUsers {
		if _, ok := currentUsers[u.TwitterID]; ok {
			currentUsers[u.TwitterID] = userToCheck{
				// Keep full and current Twitter user!
				User:             currentUsers[u.TwitterID].User,
				IsStillFollowing: true,
				IsInDatabase:     true,
			}

			continue
		}

		currentUsers[u.TwitterID] = userToCheck{
			User:             u,
			IsStillFollowing: false,
			IsInDatabase:     true,
		}
	}

	// Yay, transactions!
	tx := db.MustBegin()

	// Prepared statement to insert a new user.
	insert, err := tx.Preparex(`insert into users
		(twitter_id, screen_name, display_name, profile_icon, color)
		values (?, ?, ?, ?, ?)`)
	if err != nil {
		log.Panic(err)
	}

	// Prepared statement to update an existing follower.
	update, err := tx.Preparex(`update users
		set screen_name = ?, display_name = ?, profile_icon = ?,
		color = ? where twitter_id = ?`)
	if err != nil {
		log.Panic(err)
	}

	// Prepared statement to register a user followed.
	follow, err := tx.Preparex(`insert into events
		(token_id, user_id, event_type) values (?, ?, 'f')`)
	if err != nil {
		log.Panic(err)
	}

	// Prepared statement to register a user unfollowed.
	unfollow, err := tx.Preparex(`insert into events
		(token_id, user_id, event_type) values (?, ?, 'u')`)
	if err != nil {
		log.Panic(err)
	}

	// Go through each user
	for _, u := range currentUsers {
		var screenName, displayName, profileIcon, color string
		var twitterID int64

		// As we could have something from the database
		switch v := u.User.(type) {
		case anaconda.User:
			screenName = v.ScreenName
			twitterID = v.Id
			displayName = v.Name
			profileIcon = v.ProfileImageUrlHttps
			color = v.ProfileLinkColor
		case user: // if this, user unfollowed
			screenName = v.ScreenName
			twitterID = v.TwitterID
			displayName = v.DisplayName
			profileIcon = v.ProfileIcon
			color = v.Color
		}

		if !u.IsInDatabase { // New follower, as they were not in database
			var u user
			err := db.Get(&u, `select id from users where twitter_id = ?`,
				twitterID)

			if err == sql.ErrNoRows {
				res, err := insert.Exec(twitterID, screenName, displayName,
					profileIcon, color)
				if err != nil {
					log.Panic(err)
				}
				lastID, _ := res.LastInsertId()

				follow.Exec(tokenToFetchWith, lastID)
			} else { // Was already in DB from other user
				follow.Exec(tokenToFetchWith, u.ID)

				update.Exec(screenName, displayName, profileIcon,
					color, u.ID)
			}
		} else {
			var dbUser user
			db.Get(&dbUser, `select id from users where twitter_id = ?`,
				twitterID)

			var checkFollower event
			var hasNoEvents bool

			// Get what the last event was, so we can check if it changed.
			err := db.Get(&checkFollower, `select event_type from events t1
				join (select user_id, max(event_date) event_date
					from events where token_id = ? group by token_id, user_id) t2
					on
						t1.user_id = t2.user_id and
						t1.event_date = t2.event_date
				where t1.user_id = ?`, tokenToFetchWith, dbUser.ID)
			if err == sql.ErrNoRows {
				hasNoEvents = true
			}

			if !u.IsStillFollowing && (hasNoEvents || checkFollower.EventType == "f") { // New unfollower
				w.Write([]byte("event: unfollow\ndata: "))
				j, _ := json.Marshal(u.User)
				w.Write(j)
				w.Write([]byte("\n\n"))
				f.Flush()
				unfollow.Exec(tokenToFetchWith, dbUser.ID)
			} else if hasNoEvents || checkFollower.EventType == "u" { // New follower
				w.Write([]byte("event: follow\ndata: "))
				j, _ := json.Marshal(u.User)
				w.Write(j)
				w.Write([]byte("\n\n"))
				f.Flush()
				follow.Exec(tokenToFetchWith, dbUser.ID)
			}

			update.Exec(screenName, displayName, profileIcon,
				color, dbUser.ID)
		}
	}

	insert.Close()
	update.Close()

	follow.Close()
	unfollow.Close()

	// Commit the transaction.
	tx.Commit()

	w.Write([]byte("event: complete\ndata: Complete\n\n"))
	f.Flush()
}

func initDB() {
	db = sqlx.MustOpen("sqlite3", "unfollowers.db")

	db.MustExec(`
		create table if not exists config (
			id integer primary key,

			key text unique not null,
			value text not null
		);

		create table if not exists tokens (
			id integer primary key,

			token text not null,
			secret text not null,

			twitter_id integer not null,
			screen_name text not null,
			display_name text not null
		);

		create table if not exists users (
			id integer primary key,

			twitter_id integer not null unique,
			screen_name text not null,
			display_name text not null,

			profile_icon text,
			color text
		);

		create table if not exists events (
			id integer primary key,

			token_id integer references tokens(id),
			user_id integer references user(id),

			event_type text check (event_type in ('f', 'u')),
			event_date datetime default current_timestamp
		);
	`)
}

func main() {
	anaconda.SetConsumerKey(TwitterConsumer)
	anaconda.SetConsumerSecret(TwitterSecret)

	initDB()

	startServer()
}
