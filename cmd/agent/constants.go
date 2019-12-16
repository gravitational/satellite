package main

import "time"

// statusTimeout is the maximum time status query is blocked.
const statusTimeout = 5 * time.Second

// defaultTimelineRetention defines the default duration to store timeline events.
const defaultTimelineRentention = time.Hour * 24 * 7

// stamp defines default timestamp format.
const stamp = "Jan _2 15:04:05 UTC"

// monitoringDbFile names the file where agent persists health status history.
const monitoringDbFile = "monitoring.db"
