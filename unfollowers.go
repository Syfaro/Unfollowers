package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/ChimeraCoder/anaconda"
	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
	"github.com/skratchdot/open-golang/open"
)

//go:generate go-bindata data/

var db *sqlx.DB

const (
	TwitterConsumer = "iR0mPmyUl4IQX4ebSZGe60UpM"
	TwitterSecret   = "rFP2xPufsKa0NUWbkVuhLoJIWnhyEfJWiJ0htGKJ1Lnkd8klyr"
)

func startServer() {
	http.HandleFunc("/", accountSelect)

	http.HandleFunc("/account", accountView)
	http.HandleFunc("/known", followersKnown)
	http.HandleFunc("/latest", followersLatest)
	http.HandleFunc("/all", followersAll)
	http.HandleFunc("/tokens", tokensList)
	http.HandleFunc("/load", accountLoad)

	http.HandleFunc("/config", configFetch)
	http.HandleFunc("/config/update", configUpdate)

	http.HandleFunc("/auth", authTwitter)
	http.HandleFunc("/auth/callback", authTwitterCallback)

	http.HandleFunc("/asset", assetFetch)

	var customHost config
	err := db.Get(&customHost, `select value from config
		where key = 'custom-host'`)
	if err == sql.ErrNoRows {
		listenHost = "127.0.0.1:8080"
	} else {
		listenHost = customHost.Value
	}

	go func() {
		time.Sleep(time.Second)
		open.Run("http://" + listenHost)
	}()

	http.ListenAndServe(listenHost, nil)
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

	w.Write([]byte("event: status\ndata: Stored tokens are valid for " +
		tokenSet.ScreenName + "\n\n"))
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

		w.Write([]byte("event: status\ndata: Loaded group of users (" +
			strconv.Itoa(len(currentUsers)) + " / " + strconv.Itoa(len(all_ids)) + ")\n\n"))
		f.Flush()
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
			var dbUser user
			err := db.Get(&dbUser, `select id from users where twitter_id = ?`,
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
				follow.Exec(tokenToFetchWith, dbUser.ID)

				update.Exec(screenName, displayName, profileIcon,
					color, dbUser.ID)
			}

			w.Write([]byte("event: follow\ndata: "))
			j, _ := json.Marshal(u.User)
			w.Write(j)
			w.Write([]byte("\n\n"))
			f.Flush()
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
			} else if u.IsStillFollowing && (hasNoEvents || checkFollower.EventType == "u") { // New follower
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

	w.Write([]byte("event: complete\ndata: Completed " +
		tokenSet.ScreenName + "\n\n"))
	f.Flush()
}

func initDB() {
	db = sqlx.MustOpen("sqlite3", "unfollowers.db")

	db.MustExec(string(MustAsset("data/db_init.sql")))
}

func main() {
	anaconda.SetConsumerKey(TwitterConsumer)
	anaconda.SetConsumerSecret(TwitterSecret)

	initDB()

	resetConfig := flag.Bool("resetConfig", false,
		"Enable to reset configuration options.")
	resetAll := flag.Bool("resetAll", false,
		"Enable to reset the database.")

	flag.Parse()

	if *resetAll {
		db.MustExec(`
			drop table config;
			drop table tokens;
			drop table users;
			drop table events;`)

		initDB()
	} else if *resetConfig {
		db.MustExec(`delete from config`)
	}

	go backgroundConfigCheck()

	startServer()
}
