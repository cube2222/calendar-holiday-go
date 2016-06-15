package main

import (
	"os"
	"golang.org/x/oauth2/google"
	"golang.org/x/oauth2"
	"net/http"
	"github.com/gorilla/mux"
	"time"
	"fmt"
	"github.com/satori/go.uuid"
	"golang.org/x/net/context"
	"google.golang.org/api/calendar/v3"
	"strings"
)

var (
	googleOauthConfig = &oauth2.Config{
		RedirectURL:	"http://localhost:3000/oauthcallback",
		ClientID:     os.Getenv("googlekey"), // from https://console.developers.google.com/project/<your-project-id>/apiui/credential
		ClientSecret: os.Getenv("googlesecret"), // from https://console.developers.google.com/project/<your-project-id>/apiui/credential
		Scopes:       []string{"https://www.googleapis.com/auth/calendar"},
		Endpoint:     google.Endpoint,
	}
)

type startEnd struct {
	start time.Time
	end time.Time
}

type plannedEvent struct {
	timespan startEnd
	tags []string
}

var globalSessionDateParams map[string]startEnd

// Sorry for the placeholderish session usage of the state variable. Shouldn't be used in production. Never ever.
// 22:25-23:31
// 23:55-23:57

func main() {
	globalSessionDateParams = make(map[string]startEnd)

	m := mux.NewRouter()
	m.HandleFunc("/{start}/{end}", handleIndex)
	m.HandleFunc("/oauthcallback", handleGoogleCallback)

	http.ListenAndServe(":3000", m)
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)

	curStart, err := time.Parse("2006-Jan-02", vars["start"])
	curEnd, err := time.Parse("2006-Jan-02", vars["end"])

	if err != nil {
		fmt.Fprint(w, err)
		return
	}

	curStartEnd := startEnd{start: curStart, end: curEnd}

	curSession := uuid.NewV4().String()

	globalSessionDateParams[curSession] = curStartEnd // Save session params so that when the user logs in he'll get his answer.

	url := googleOauthConfig.AuthCodeURL(curSession)

	http.Redirect(w, r, url, http.StatusTemporaryRedirect)
}

func handleGoogleCallback(w http.ResponseWriter, r *http.Request) {
	state := r.FormValue("state")

	curStartEnd, ok := globalSessionDateParams[state] // Get params from session

	if ok == false {
		fmt.Fprint(w, "Your Oauth2 state has not been found.")
		return
	}

	code := r.FormValue("code")
	token, err := googleOauthConfig.Exchange(context.Background(), code)
	if err != nil {
		fmt.Fprint(w, "Oauth2 exchnage failed.")
		return
	}

	client := oauth2.NewClient(context.Background(), oauth2.StaticTokenSource(token))

	calendarService, err := calendar.New(client)
	if err != nil {
		fmt.Fprint(w, "Couldn't create calendar service.")
		return
	}
	// Get the events
	eventList, err := calendarService.Events.List("primary").TimeMin(curStartEnd.start.Format(time.RFC3339)).TimeMax(curStartEnd.end.Format(time.RFC3339)).Do()
	if err != nil {
		fmt.Fprint(w, err)
		return
	}


	allEventList := make([]*calendar.Event, 0, 30)

	for _, item := range eventList.Items {
		allEventList = append(allEventList, item)
	}
	// If there are more pages, get all events from there
	for eventList.NextPageToken != "" {
		eventList, err = calendarService.Events.List("primary").
			TimeMin(curStartEnd.start.Format(time.RFC3339)).
			TimeMax(curStartEnd.end.Format(time.RFC3339)).
			PageToken(eventList.NextPageToken).
			Do()
		if err != nil {
			fmt.Fprint(w, err)
			return
		}

		for _, item := range eventList.Items {
			allEventList = append(allEventList, item)
		}
	}

	plannedEventList := make([]plannedEvent, 0, 30)

	for _, item := range allEventList {
		if item.Start.DateTime == "" { // Means it's all day
			curStart, err := time.Parse("2006-01-02", item.Start.Date)
			curEnd, err := time.Parse("2006-01-02", item.End.Date)
			if err != nil {
				fmt.Fprint(w, "Error parsing dates.")
				return
			}
			curTimespan := startEnd{start: curStart, end: curEnd}
			curPlannedEvent := plannedEvent{timespan: curTimespan, tags: getTags(item.Summary)}
			if len(curPlannedEvent.tags) > 0 {
				plannedEventList = append(plannedEventList, curPlannedEvent)
			}
		}
	}

	holidaysLeft := 26

	for _, item := range plannedEventList {
		holiday := false
		for _, item := range item.tags {
			if item == "holiday" || item == "urlop" {
				holiday = true
			}
		}
		if holiday {
			holidaysLeft = holidaysLeft - int(item.timespan.end.Sub(item.timespan.start).Hours() / 24)
		}
	}

	fmt.Fprintln(w, holidaysLeft)
}

func getTags(text string) []string {
	splitStrings := strings.Split(text, "#")
	tags := make([]string, 0, 5)
	if text[0] == '#' {
		for index, item := range splitStrings {
			if index % 2 == 0 {
				tags = append(tags, item)
			}
		}
	} else {
		for index, item := range splitStrings {
			if index % 2 == 1 {
				tags = append(tags, item)
			}
		}
	}
	return tags
}