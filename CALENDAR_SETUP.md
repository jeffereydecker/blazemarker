# Calendar Integration Setup Guide

## Overview
Blazemarker now integrates with your existing Radicale CalDAV server to display a family calendar. All family members can view events through Blazemarker, while events are stored in your jdecker calendar.

## Setup Steps

### 1. Configure CalDAV Credentials
Edit `/config/caldav.conf` and update with your Radicale details:

```bash
CALDAV_SERVER_URL=http://localhost:5232
CALDAV_USERNAME=jdecker
CALDAV_PASSWORD=your_actual_radicale_password
CALDAV_CALENDAR=
```

**Important:** 
- Replace `your_actual_radicale_password` with your real password
- If Radicale is on a different host/port, update the URL
- Leave `CALDAV_CALENDAR` empty to use your default calendar

### 2. Verify Radicale is Running
Make sure your Radicale server is accessible:
```bash
curl http://localhost:5232/jdecker/
```

### 3. Restart Blazemarker
The calendar integration loads on startup:
```bash
cd /Users/jdecker/go/blazemarker
pkill -f "./index"
cd index && ./index &
```

### 4. Access the Calendar
- Navigate to **http://localhost:3000/calendar**
- Or click "Calendar" in the navbar

## Features

### Current Features
✅ **View calendar events** - Month view with all events from your Radicale calendar
✅ **Click-to-view day events** - Click any day to see events for that specific date
✅ **Add events** - Create single events or recurring series (daily, weekly, monthly)
✅ **Delete events** - Remove individual events or entire recurring series
✅ **Event details modal** - Click any event to see full details with delete options
✅ **Month navigation** - Browse previous and future months
✅ **All-day event support** - Properly displays all-day vs. timed events
✅ **Mobile responsive** - Compact view for smaller screens with dots on very small displays
✅ **Multi-user access** - All family members can view and manage the calendar
✅ **Recurring events** - Support for daily, weekly, and monthly recurring events

### How It Works
- **Events stored in Radicale** - Your jdecker calendar is the source of truth
- **Sync with calendar apps** - Events added in Apple Calendar, Google Calendar, etc. appear in Blazemarker
- **Full CRUD** - Create, read, update (coming soon), and delete events through Blazemarker

## Using the Calendar

### Adding Events

1. **Click "+ Add Event"** button in the calendar header
2. **Fill in event details**:
   - **Title** (required)
   - **Description** (optional)
   - **Location** (optional)
   - **All Day** checkbox - toggles between date and datetime inputs
   - **Start time** (required)
   - **End time** (optional, defaults to 1 hour after start)
   - **Repeat** - Create recurring events:
     - Daily for 7 or 14 days
     - Weekly for 4 or 8 weeks
     - Monthly for 3, 6, or 12 months
3. **Click "Create Event"** to save

### Viewing Events

- **Calendar Grid**: See all events in month view
- **Click any day**: View all events for that specific date
- **Click an event**: Open detailed modal with full information
- **Close day view**: Click × button to hide day events

### Deleting Events

1. **Click an event** to open the details modal
2. **Choose delete option**:
   - **Delete Event** - Removes only this single event
   - **Delete Series** - Removes all events in recurring series (if applicable)
3. **Confirm** the deletion
4. Calendar automatically refreshes

## Event Display

### Month View
- **Grid layout** - 7-column calendar grid (Sun-Sat)
- **Color coding**:
  - Purple gradient: Regular timed events
  - Green gradient: All-day events
- **Today highlight** - Current day has blue background
- **Other month days** - Grayed out for context

### Upcoming Events
- Shows next 30 days of events
- Card-based layout with:
  - Event title
  - Date/time
  - Location (if specified)
  - Description

### Event Details Modal
Click any event to see:
- Full title
- Start/end time (or all-day indicator)
- Location
- Full description

## Future Enhancements

### Planned Features
- ✏️ **Edit events** - Modify existing events inline
- 👥 **Event RSVPs** - Track which family members are attending
- 🔗 **Link to photos/articles** - Associate events with content
- 🔔 **Event reminders** - Email/push notifications for upcoming events
- 📅 **Week/day views** - Additional calendar views
- 🎨 **Color categories** - Different colors for event types
- 🔄 **Advanced recurring rules** - Custom RRULE support for complex patterns

## Technical Details

### Architecture
- **Backend**: Go with CalDAV client (`github.com/emersion/go-webdav`)
- **Calendar standard**: RFC 4791 (CalDAV), RFC 5545 (iCalendar)
- **Storage**: Events stored in Radicale's SQLite/filesystem
- **Auth**: Uses jdecker's credentials for CalDAV access
- **Template**: Bootstrap 5 responsive design

### Files Added
- `/calendar_db/calendar_db.go` - CalDAV client and event fetching
- `/calendar_db/go.mod` - Calendar module dependencies
- `/templates/calendar.html` - Calendar view template
- `/config/caldav.conf` - CalDAV configuration
- `/index/index.go` - Added servCalendar handler and /calendar route

### Module Structure
```
calendar_db/
├── calendar_db.go      # CalDAV integration
│   ├── GetCalendarEvents()  # Fetch events from Radicale
│   ├── CreateEvent()        # Add new event (future use)
│   └── basicAuthTransport   # HTTP auth for CalDAV
└── go.mod              # Dependencies
```

## Troubleshooting

### Calendar Not Loading
1. **Check Radicale is running**:
   ```bash
   ps aux | grep radicale
   curl http://localhost:5232/
   ```

2. **Verify credentials**:
   - Ensure caldav.conf has correct password
   - Test login manually:
     ```bash
     curl -u jdecker:password http://localhost:5232/jdecker/
     ```

3. **Check Blazemarker logs**:
   ```bash
   tail -f /Users/jdecker/go/blazemarker/logs/blazemarker.log
   ```
   Look for errors like:
   - "Failed to create CalDAV client"
   - "Failed to find calendars"

### No Events Showing
1. **Verify events exist** in Radicale:
   - Check Apple Calendar or other CalDAV client
   - Ensure events are in date range being queried

2. **Check calendar path**:
   - Radicale may use different calendar names
   - Try leaving `CALDAV_CALENDAR` empty to use first calendar

### Permission Denied
- Ensure caldav.conf file is readable
- Check file permissions:
  ```bash
  chmod 600 /Users/jdecker/go/blazemarker/config/caldav.conf
  ```

## Security Notes

### Credential Protection
- **caldav.conf contains passwords** - Add to .gitignore
- **File permissions**: `chmod 600 caldav.conf`
- **Consider encryption**: Use environment variables instead for production

### Access Control
- All authenticated Blazemarker users can view calendar
- Calendar writes still use jdecker's credentials
- Events created in Blazemarker will show jdecker as creator in Radicale

## Next Steps

1. **Configure caldav.conf** with your password
2. **Restart Blazemarker** to load configuration
3. **Visit /calendar** to see your events
4. **Test with existing events** from your calendar apps
5. **Add new events** via Apple Calendar/Google Calendar to verify sync

Enjoy your integrated family calendar! 📅
