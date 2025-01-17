package exporter

import (
	"log"
	"time"
)

type VehicleCollector struct {
	FRMAddress      string
	TrackedVehicles map[string]*VehicleDetails
}

type VehicleDetails struct {
	Name   		  string   `json:"Name"`
	Location      Location `json:"location"`
	ForwardSpeed  float64  `json:"ForwardSpeed"`
	AutoPilot     bool     `json:"AutoPilot"`
	FuelType      string   `json:"FuelType"`
	FuelInventory float64  `json:"FuelInventory"`
	PathName      string   `json:"PathName"`
	DepartTime    time.Time
	Departed      bool
}

func (v *VehicleDetails) recordElapsedTime() {
	now := Clock.Now()
	tripSeconds := now.Sub(v.DepartTime).Seconds()
	VehicleRoundTrip.WithLabelValues(v.Name, v.PathName).Set(tripSeconds)
	v.Departed = false
}

func (v *VehicleDetails) isCompletingTrip(updatedLocation Location) bool {
	// vehicle near first tracked location facing roughly the same way
	return v.Departed && v.Location.isNearby(updatedLocation) && v.Location.isSameDirection(updatedLocation)
}

func (v *VehicleDetails) isStartingTrip(updatedLocation Location) bool {
	// vehicle departed from first tracked location
	return !v.Departed && !v.Location.isNearby(updatedLocation)
}

func (v *VehicleDetails) startTracking(trackedVehicles map[string]*VehicleDetails) {
	// Only start tracking the vehicle at low speeds so it's
	// likely at a station or somewhere easier to track.
	if v.ForwardSpeed < 10 {
		trackedVehicle := VehicleDetails{
			Location:    v.Location,
			Name: 		 v.Name,
			PathName:    v.PathName,
			Departed:    false,
		}
		trackedVehicles[v.Name] = &trackedVehicle
	}
}

func (d *VehicleDetails) handleTimingUpdates(trackedVehicles map[string]*VehicleDetails) {
	if d.AutoPilot {
		vehicle, exists := trackedVehicles[d.Name]
		if exists && vehicle.isCompletingTrip(d.Location) {
			vehicle.recordElapsedTime()
		} else if exists && vehicle.isStartingTrip(d.Location) {
			vehicle.Departed = true
			vehicle.DepartTime = Clock.Now()
		} else if !exists {
			d.startTracking(trackedVehicles)
		}
	} else {
		//remove manual vehicles, nothing to mark
		_, exists := trackedVehicles[d.Name]
		if exists {
			delete(trackedVehicles, d.Name)
		}
	}
}

func NewVehicleCollector(frmAddress string) *VehicleCollector {
	return &VehicleCollector{
		FRMAddress:      frmAddress,
		TrackedVehicles: make(map[string]*VehicleDetails),
	}
}

func (c *VehicleCollector) Collect() {
	details := []VehicleDetails{}
	err := retrieveData(c.FRMAddress, &details)
	if err != nil {
		log.Printf("error reading vehicle statistics from FRM: %s\n", err)
		return
	}

	for _, d := range details {
		VehicleFuel.WithLabelValues(d.Name, d.FuelType).Set(d.FuelInventory)

		d.handleTimingUpdates(c.TrackedVehicles)
	}
}
