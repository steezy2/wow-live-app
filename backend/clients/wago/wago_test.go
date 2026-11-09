package wago

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMapBoundsReadsTheFirstRowOfEachMapAsked(t *testing.T) {
	csv := "UiMin_0,Region_0,Region_1,Region_2,Region_3,Region_4,Region_5,ID,UiMapID\n" +
		"0,980,-581.25,-1000000,1755,581.25,1000000,69578,2610\n" +
		"0,1,2,3,4,5,6,69579,2610\n" +
		"0,4610.4,-12325,-1000000,4939.6,-11831.3,1000000,69654,2632\n" +
		"0,1,2,3,4,5,6,1,9999\n"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, csv) }))
	defer server.Close()
	got, err := Client{CacheDir: t.TempDir(), Base: server.URL}.MapBounds(context.Background(), []int{2610, 2632})
	if err != nil || len(got) != 2 || fmt.Sprint(got[2610]) != "{2610 980 1755 -581.25 581.25}" ||
		got[2632].MaxY != -11831.3 {
		t.Fatalf("bounds = %v (%v); want Ula'tek's and Nymrissa's rooms from their first rows, not map 9999", got, err)
	}
}
