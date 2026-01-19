package calendar_db

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/emersion/go-ical"
	"github.com/emersion/go-webdav/caldav"
	"github.com/jeffereydecker/blazemarker/blaze_log"
)

// debugICalString returns a string representation of the iCalendar object for debugging
func debugICalString(cal *ical.Calendar) string {
	var sb strings.Builder
	sb.WriteString("BEGIN:VCALENDAR\n")
	for _, prop := range cal.Props {
		sb.WriteString(fmt.Sprintf("%+v\n", prop))
	}
	for _, comp := range cal.Children {
		sb.WriteString(debugComponentString(comp))
	}
	sb.WriteString("END:VCALENDAR\n")
	return sb.String()
}

func debugComponentString(comp *ical.Component) string {
	var sb strings.Builder
	sb.WriteString("BEGIN:" + comp.Name + "\n")
	for _, prop := range comp.Props {
		sb.WriteString(fmt.Sprintf("%+v\n", prop))
	}
	for _, child := range comp.Children {
		sb.WriteString(debugComponentString(child))
	}
	sb.WriteString("END:" + comp.Name + "\n")
	return sb.String()
}

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
	RRule       string // Recurrence rule (RRULE)
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

			// Check for recurrence rule
			rruleProp := component.Props.Get(ical.PropRecurrenceRule)
			if rruleProp != nil {
				// Get EXDATE properties (exception dates)
				var exdates []time.Time
				exdateProps := component.Props["EXDATE"]
				for _, exdateProp := range exdateProps {
					// Parse EXDATE value (format: 20260128 or 20260128T220000Z)
					if t, err := time.Parse("20060102T150405Z", exdateProp.Value); err == nil {
						exdates = append(exdates, t)
					} else if t, err := time.Parse("20060102", exdateProp.Value); err == nil {
						exdates = append(exdates, t)
					}
				}

				// This is a recurring event - expand it
				expandedEvents := expandRecurringEvent(event, rruleProp.Value, startDate, endDate, exdates)
				events = append(events, expandedEvents...)
			} else {
				// Single event
				events = append(events, event)
			}
		}
	}

	// Sort events by start time
	sort.Slice(events, func(i, j int) bool {
		return events[i].StartTime.Before(events[j].StartTime)
	})

	logger.Info("Fetched calendar events", "count", len(events))
	return events, nil
}

// expandRecurringEvent expands a recurring event based on RRULE into individual instances
func expandRecurringEvent(baseEvent Event, rrule string, startDate, endDate time.Time, exdates []time.Time) []Event {
	var events []Event

	// Parse simple RRULE patterns
	// Format examples: "FREQ=WEEKLY;COUNT=52", "FREQ=DAILY;UNTIL=20260101", "FREQ=MONTHLY"
	freq := ""
	count := 365 // Default max occurrences
	interval := 1
	var until time.Time

	// Unescape the RRULE (handle \; -> ;)
	rrule = strings.ReplaceAll(rrule, "\\;", ";")
	rrule = strings.ReplaceAll(rrule, "\\,", ",")

	// Simple RRULE parser
	parts := strings.Split(rrule, ";")
	for _, part := range parts {
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			continue
		}
		key := strings.TrimSpace(kv[0])
		value := strings.TrimSpace(kv[1])

		switch key {
		case "FREQ":
			freq = value
		case "COUNT":
			fmt.Sscanf(value, "%d", &count)
		case "INTERVAL":
			fmt.Sscanf(value, "%d", &interval)
		case "UNTIL":
			// Parse UNTIL date (format: 20260101T000000Z or 20260101)
			if t, err := time.Parse("20060102T150405Z", value); err == nil {
				until = t
			} else if t, err := time.Parse("20060102", value); err == nil {
				until = t
			}
		}
	}

	// Limit count to prevent infinite loops
	if count > 1000 {
		count = 1000
	}

	duration := baseEvent.EndTime.Sub(baseEvent.StartTime)
	currentTime := baseEvent.StartTime

	// Generate occurrences
	for i := 0; i < count; i++ {
		// Check if we're past the end date or UNTIL date
		if currentTime.After(endDate) {
			break
		}
		if !until.IsZero() && currentTime.After(until) {
			break
		}

		// Only include events within our query range
		if !currentTime.Before(startDate) {
			// Check if this date is excluded (EXDATE)
			isExcluded := false
			for _, exdate := range exdates {
				// Compare just the date part (year, month, day)
				if currentTime.Year() == exdate.Year() &&
					currentTime.Month() == exdate.Month() &&
					currentTime.Day() == exdate.Day() {
					isExcluded = true
					break
				}
			}

			if !isExcluded {
				occurrence := baseEvent
				occurrence.StartTime = currentTime
				occurrence.EndTime = currentTime.Add(duration)
				// Make UID unique for each occurrence
				occurrence.UID = fmt.Sprintf("%s-%s", baseEvent.UID, currentTime.Format("20060102"))
				events = append(events, occurrence)
			}
		}

		// Advance to next occurrence
		switch freq {
		case "DAILY":
			currentTime = currentTime.AddDate(0, 0, interval)
		case "WEEKLY":
			currentTime = currentTime.AddDate(0, 0, 7*interval)
		case "MONTHLY":
			currentTime = currentTime.AddDate(0, interval, 0)
		case "YEARLY":
			currentTime = currentTime.AddDate(interval, 0, 0)
		default:
			// Unknown frequency, stop
			logger.Warn("Unknown RRULE frequency", "freq", freq, "rrule", rrule)
			logger.Debug("Expanded recurring event", "title", baseEvent.Title, "occurrences", len(events))
			return events
		}
	}

	logger.Debug("Expanded recurring event", "title", baseEvent.Title, "occurrences", len(events))
	return events
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
		// Set DTSTART and DTEND as floating times (no timezone)
		dtstartProp := ical.NewProp(ical.PropDateTimeStart)
		dtstartProp.Value = event.StartTime.Format("20060102T150405")
		vevent.Props.Set(dtstartProp)

		dtendProp := ical.NewProp(ical.PropDateTimeEnd)
		dtendProp.Value = event.EndTime.Format("20060102T150405")
		vevent.Props.Set(dtendProp)

		if event.RRule != "" {
			// Add RRULE without VALUE=TEXT
			vevent.Props.Set(&ical.Prop{Name: ical.PropRecurrenceRule, Value: event.RRule})
			logger.Info("Added RRULE to event", "uid", event.UID, "rrule", event.RRule)
		}
	}

	vevent.Props.SetDateTime(ical.PropDateTimeStamp, time.Now().UTC())

	calendar.Children = append(calendar.Children, vevent)

	// Debug: log the iCalendar data before upload
	logger.Debug("iCalendar data before upload", "ical", debugICalString(calendar))
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

// UpdateEvent modifies an existing event on the CalDAV server
func UpdateEvent(config CalendarConfig, uid string, calendar *ical.Calendar) error {
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

	// Update calendar object
	path := fmt.Sprintf("%s/%s.ics", targetCalendar.Path, uid)
	_, err = client.PutCalendarObject(ctx, path, calendar)
	if err != nil {
		logger.Error("Failed to update calendar event", "error", err, "uid", uid)
		return fmt.Errorf("failed to update event: %w", err)
	}

	logger.Info("Updated calendar event", "uid", uid)
	return nil
}

// DeleteEvent removes an event or adds an exception date for recurring events
func DeleteEvent(config CalendarConfig, uid string, deleteSeries bool, instanceDate time.Time) error {
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

	// Extract original UID if this is a recurring event occurrence
	// Format: "originalUID-20260128" -> "originalUID"
	originalUID := uid
	if idx := strings.LastIndex(uid, "-"); idx > 0 {
		// Check if the part after the dash looks like a date (8 digits)
		datePart := uid[idx+1:]
		if len(datePart) == 8 {
			// Validate it's a numeric date
			allDigits := true
			for _, c := range datePart {
				if c < '0' || c > '9' {
					allDigits = false
					break
				}
			}
			if allDigits {
				originalUID = uid[:idx]
				logger.Info("Detected recurring event occurrence", "provided", uid, "original", originalUID, "deleteSeries", deleteSeries)
			}
		}
	}

	// If not deleting the series and we have an instance date, add EXDATE instead
	if !deleteSeries && !instanceDate.IsZero() && originalUID != uid {
		// Fetch the event
		path := fmt.Sprintf("%s/%s.ics", targetCalendar.Path, originalUID)
		data, err := client.GetCalendarObject(ctx, path)
		if err != nil {
			logger.Error("Failed to fetch calendar event for EXDATE update", "error", err, "uid", originalUID)
			return fmt.Errorf("failed to fetch event: %w", err)
		}

		// data.Data is already an *ical.Calendar
		cal := data.Data

		// Find the VEVENT component
		var eventComponent *ical.Component
		for _, child := range cal.Children {
			if child.Name == ical.CompEvent {
				eventComponent = child
				break
			}
		}

		if eventComponent == nil {
			return fmt.Errorf("no VEVENT component found")
		}

		// Add EXDATE property
		// Match EXDATE format to DTSTART (date for all-day, datetime for timed)
		dtstart := eventComponent.Props.Get(ical.PropDateTimeStart)
		exdateValue := ""
		if dtstart != nil {
			if len(dtstart.Value) == 8 {
				// All-day event (YYYYMMDD)
				exdateValue = instanceDate.Format("20060102")
			} else {
				// Timed event (YYYYMMDDTHHMMSSZ)
				exdateValue = instanceDate.UTC().Format("20060102T150405Z")
			}
		} else {
			// Fallback: use date only
			exdateValue = instanceDate.Format("20060102")
		}

		exdateProp := ical.NewProp("EXDATE")
		exdateProp.Value = exdateValue
		eventComponent.Props.Add(exdateProp)

		logger.Info("Adding EXDATE to recurring event", "uid", originalUID, "exdate", exdateValue)
		logger.Debug("iCalendar data before update", "ical", debugICalString(cal))

		// Update the event
		err = UpdateEvent(config, originalUID, cal)
		if err != nil {
			return err
		}

		logger.Info("Added EXDATE to recurring event", "uid", originalUID, "instance", instanceDate.Format("2006-01-02"))
		return nil
	}

	// Delete the entire series or a non-recurring event
	path := fmt.Sprintf("%s/%s.ics", targetCalendar.Path, originalUID)
	err = client.RemoveAll(ctx, path)
	if err != nil {
		logger.Error("Failed to delete calendar event", "error", err, "uid", uid, "originalUID", originalUID)
		return fmt.Errorf("failed to delete event: %w", err)
	}

	logger.Info("Deleted calendar event", "uid", uid, "originalUID", originalUID, "deleteSeries", deleteSeries)
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
