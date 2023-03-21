package exporter

import (
	"log"
	"time"
)

type TrainCollector struct {
	FRMAddress    string
	TrackedTrains map[string]*TrainDetails
}

type TimeTable struct {
	StationName string `json:"StationName"`
}
type TrainDetails struct {
	Name        string      `json:"Name"`
	PowerConsumed    float64     `json:"PowerConsumed"`
	TrainStation     string      `json:"TrainStation"`
	Derailed         bool        `json:"Derailed"`
	Status           string      `json:"Status"` //"TS_SelfDriving",
	TimeTable        []TimeTable `json:"TimeTable"`
	ArrivalTime      time.Time
	StationCounter   int
	FirstArrivalTime time.Time
}

func NewTrainCollector(frmAddress string) *TrainCollector {
	return &TrainCollector{
		FRMAddress:    frmAddress,
		TrackedTrains: make(map[string]*TrainDetails),
	}
}

func (t *TrainDetails) recordRoundTripTime(now time.Time) {
	if len(t.TimeTable) <= t.StationCounter {
		roundTripSeconds := now.Sub(t.FirstArrivalTime).Seconds()
		TrainRoundTrip.WithLabelValues(t.Name).Set(roundTripSeconds)
		t.StationCounter = 0
		t.FirstArrivalTime = now
	}
}

func (t *TrainDetails) recordSegmentTripTime(destination string, now time.Time) {
	tripSeconds := now.Sub(t.ArrivalTime).Seconds()
	TrainSegmentTrip.WithLabelValues(t.Name, t.TrainStation, destination).Set(tripSeconds)
}

func (t *TrainDetails) recordNextStation(d *TrainDetails) {
	if t.TrainStation != d.TrainStation {
		t.StationCounter = t.StationCounter + 1
		now := Clock.Now()
		t.recordSegmentTripTime(d.TrainStation, now)
		t.recordRoundTripTime(now)
		t.ArrivalTime = now
		t.TrainStation = d.TrainStation
	}
}

func (t *TrainDetails) markFirstStation(d *TrainDetails) {
	if t.TrainStation != d.TrainStation {
		t.StationCounter = 0
		t.FirstArrivalTime = Clock.Now()
		t.ArrivalTime = Clock.Now()
		t.TrainStation = d.TrainStation
	}
}

func (t *TrainDetails) startTracking(trackedTrains map[string]*TrainDetails) {
	trackedTrain := TrainDetails{
		Name:      t.Name,
		TrainStation:   t.TrainStation,
		StationCounter: 0,
		TimeTable:      t.TimeTable,
	}
	trackedTrains[t.Name] = &trackedTrain
}

func (d *TrainDetails) handleTimingUpdates(trackedTrains map[string]*TrainDetails) {
	// track self driving train timing
	if d.Status == "TS_SelfDriving" {
		train, exists := trackedTrains[d.Name]
		if exists && !train.FirstArrivalTime.IsZero() {
			train.recordNextStation(d)
		} else if exists {
			train.markFirstStation(d)
		} else {
			d.startTracking(trackedTrains)
		}
	} else {
		//remove manual trains, nothing to mark
		_, exists := trackedTrains[d.Name]
		if exists {
			delete(trackedTrains, d.Name)
		}
	}
}

func (c *TrainCollector) Collect() {
	details := []TrainDetails{}
	err := retrieveData(c.FRMAddress, &details)
	if err != nil {
		log.Printf("error reading train statistics from FRM: %s\n", err)
		return
	}

	for _, d := range details {
		TrainPower.WithLabelValues(d.Name).Set(d.PowerConsumed)

		isDerailed := parseBool(d.Derailed)
		TrainDerailed.WithLabelValues(d.Name).Set(isDerailed)

		d.handleTimingUpdates(c.TrackedTrains)
	}
}
