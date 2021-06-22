package handler

import (
	"fmt"
	"testing"
	"time"
)

func TestDate(t *testing.T) {
	path := "/mnt/mmcb123"
	id := "Airsence-123"
	DATE := "2021-04-01T00:00:00+00:00"
	date := time.Now().UTC()
	fmt.Printf(fmt.Sprintf(
		"%v/%v_%v.db",
		path,
		id,
		date.Format("200601"),
	))
	mileStoneTime, _ := time.Parse(
		time.RFC3339,
		DATE,
	)
	fmt.Printf(fmt.Sprintf(
		"%v/%v_%v.db",
		path,
		id,
		mileStoneTime.Format("200601"),
	))
	fmt.Println(mileStoneTime.Before(date))
}
