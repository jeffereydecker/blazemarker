package calendar_db

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/emersion/go-ical"
	"github.com/emersion/go-webdav/caldav"
	"github.com/jeffereydecker/blazemarker/blaze_log"
)

var logger = blaze_log.GetLogger()

// CalendarConfig holds the CalDAV connection details
type CalendarConfig struct {
	ServerURL string
	Username  string
	Password  string
	Calendar  string // Calendar name/path
}

// Event represents a calendar event
type Event struct {
	UID         string
	Title       string
	Description string
	Location    string
	StartTime   time.Time
	EndTime     time.Time
	AllDay      bool
	CreatedBy   string // Blazemarker username who created it
	Attendees   []string
}

// GetCalendarEvents fetches events from CalDAV server
func GetCalendarEvents(config CalendarConfig, startDate, endDate time.Time) ([]Event, error) {
	// Create HTTP client with basic auth
	httpClient := &http.Client{
		Transport: &basicAuthTransport{
			Username: config.Username,
			Password: config.Password,
		},
	}

	// Create CalDAV client
	client, err := caldav.NewClient(httpClient, config.ServerURL)
	if err != nil {
		logger.Error("Failed to create CalDAV client", "error", err)
		return nil, fmt.Errorf("failed to create CalDAV client: %w", err)
	}

	// Find calendar home
	ctx := context.Background()
	principal, err := client.FindCurrentUserPrincipal(ctx)
	if err != nil {
		logger.Error("Failed to find user principal", "error", err)
		return nil, fmt.Errorf("failed to find user principal: %w", err)
	}

	homeSet, err := client.FindCalendarHomeSet(ctx, principal)
	if err != nil {
		logger.Error("Failed to find calendar home set", "error", err)
		return nil, fmt.Errorf("failed to find calendar home set: %w", err)
	}

	// List calendars
	calendars, err := client.FindCalendars(ctx, homeSet)
	if err != nil {
		logger.Error("Failed to find calendars", "error", err)
		return nil, fmt.Errorf("failed to find calendars: %w", err)
	}

	if len(calendars) == 0 {
		return []Event{}, nil
	}

	// Use first calendar if no specific calendar specified
	var targetCalendar caldav.Calendar
	if config.Calendar != "" {
		for _, cal := range calendars {
			if cal.Name == config.Calendar {
				targetCalendar = cal
				break
			}
		}
		if targetCalendar.Path == "" {
			logger.Warn("Calendar not found, using first calendar", "requested", config.Calendar)
			targetCalendar = calendars[0]
		}
	} else {
		targetCalendar = calendars[0]
	}

	// Query calendar objects
	query := caldav.CalendarQuery{
		CompRequest: caldav.CalendarCompRequest{
			Name: "VCALENDAR",
			Comps: []caldav.CalendarCompRequest{{
				Name: "VEVENT",
				Props: []string{
					"SUMMARY",
					"DESCRIPTION",
					"LOCATION",
					"DTSTART",
					"DTEND",
					"UID",
					"ATTENDEE",
				},
			}},
		},
	}

	// Add time range filter
	query.CompFilter.Name = "VCALENDAR"
	query.CompFilter.Comps = []caldav.CompFilter{{
		Name:  "VEVENT",
		Start: startDate,
		End:   endDate,
	}}

	calendarObjects, err := client.QueryCalendar(ctx, targetCalendar.Path, &query)
	if err != nil {
		logger.Error("Failed to query calendar", "error", err)
		return nil, fmt.Errorf("failed to query calendar: %w", err)
	}

	// Parse calendar objects into events
	var events []Event
	for _, obj := range calendarObjects {
		if obj.Data == nil {
			continue
		}

		calendar := obj.Data

		for _, component := range calendar.Children {
			if component.Name != ical.CompEvent {
				continue
			}

			event := Event{}

			// Get UID
			if prop := component.Props.Get(ical.PropUID); prop != nil {
				event.UID = prop.Value
			}

			// Get title
			if prop := component.Props.Get(ical.PropSummary); prop != nil {
				event.Title = prop.Value
			}

			// Get description
			if prop := component.Props.Get(ical.PropDescription); prop != nil {
				event.Description = prop.Value
			}

			// Get location
			if prop := component.Props.Get(ical.PropLocation); prop != nil {
				event.Location = prop.Value
			}

			// Get start time
			if prop := component.Props.Get(ical.PropDateTimeStart); prop != nil {
				if t, err := prop.DateTime(time.Local); err == nil {
					event.StartTime = t
					// Check if it's an all-day event
					if prop.Params.Get(ical.ParamValue) == "DATE" {
						event.AllDay = true
					}
				}
			}

			// Get end time
			if prop := component.Props.Get(ical.PropDateTimeEnd); prop != nil {
				if t, err := prop.DateTime(time.Local); err == nil {
					event.EndTime = t
				}
			}

			// Get attendees
			attendees := component.Props[ical.PropAttendee]
			for _, prop := range attendees {
				event.Attendees = append(event.Attendees, prop.Value)
			}

			events = append(events, event)
		}
	}

	// Sort events by start time
	sort.Slice(events, func(i, j int) bool {
		return events[i].StartTime.Before(events[j].StartTime)
	})

	logger.Info("Fetched calendar events", "count", len(events))
	return events, nil
}

// CreateEvent adds a new event to the CalDAV calendar
func CreateEvent(config CalendarConfig, event Event) error {
	// Create HTTP client with basic auth
	httpClient := &http.Client{
		Transport: &basicAuthTransport{
			Username: config.Username,
			Password: config.Password,
		},
	}

	// Create CalDAV client
	client, err := caldav.NewClient(httpClient, config.ServerURL)
	if err != nil {
		logger.Error("Failed to create CalDAV client", "error", err)
		return fmt.Errorf("failed to create CalDAV client: %w", err)
	}

	ctx := context.Background()

	// Find calendar home
	principal, err := client.FindCurrentUserPrincipal(ctx)
	if err != nil {
		return fmt.Errorf("failed to find user principal: %w", err)
	}

	homeSet, err := client.FindCalendarHomeSet(ctx, principal)
	if err != nil {
		return fmt.Errorf("failed to find calendar home set: %w", err)
	}

	// List calendars
	calendars, err := client.FindCalendars(ctx, homeSet)
	if err != nil {
		return fmt.Errorf("failed to find calendars: %w", err)
	}

	if len(calendars) == 0 {
		return fmt.Errorf("no calendars found")
	}

	targetCalendar := calendars[0]

	// Create iCalendar event
	calendar := ical.NewCalendar()
	calendar.Props.SetText(ical.PropVersion, "2.0")
	calendar.Props.SetText(ical.PropProductID, "-//Blazemarker//Calendar//EN")

	vevent := ical.NewComponent(ical.CompEvent)

	// Generate UID if not provided
	if event.UID == "" {
		event.UID = fmt.Sprintf("%d@blazemarker.com", time.Now().UnixNano())
	}
	vevent.Props.SetText(ical.PropUID, event.UID)
	vevent.Props.SetText(ical.PropSummary, event.Title)

	if event.Description != "" {
		vevent.Props.SetText(ical.PropDescription, event.Description)
	}

	if event.Location != "" {
		vevent.Props.SetText(ical.PropLocation, event.Location)
	}

	// Set start and end times
	if event.AllDay {
		vevent.Props.SetDate(ical.PropDateTimeStart, event.StartTime)
		vevent.Props.SetDate(ical.PropDateTimeEnd, event.EndTime)
	} else {
		vevent.Props.SetDateTime(ical.PropDateTimeStart, event.StartTime)
		vevent.Props.SetDateTime(ical.PropDateTimeEnd, event.EndTime)
	}

	vevent.Props.SetDateTime(ical.PropDateTimeStamp, time.Now())

	calendar.Children = append(calendar.Children, vevent)

	// Put calendar object
	path := fmt.Sprintf("%s/%s.ics", targetCalendar.Path, event.UID)
	_, err = client.PutCalendarObject(ctx, path, calendar)
	if err != nil {
		logger.Error("Failed to create calendar event", "error", err)
		return fmt.Errorf("failed to create event: %w", err)
	}

	logger.Info("Created calendar event", "uid", event.UID, "title", event.Title)
	return nil
}

// basicAuthTransport implements HTTP basic authentication
type basicAuthTransport struct {
	Username string
	Password string
}

func (t *basicAuthTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.SetBasicAuth(t.Username, t.Password)
	return http.DefaultTransport.RoundTrip(req)
}
