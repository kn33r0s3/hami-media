package main

import (
	"reflect"
	"testing"
)

func TestPublisherMediaTracksReturnsSortedUniqueTracks(t *testing.T) {
	router := &sfuRouter{
		tracks: map[sfuTrackKey]*sfuPublishedTrack{
			{PublisherID: "publisher-1", TrackID: "video", Layer: "0"}: {kind: "video"},
			{PublisherID: "publisher-1", TrackID: "video", Layer: "1"}: {kind: "video"},
			{PublisherID: "publisher-1", TrackID: "audio"}:             {kind: "audio"},
			{PublisherID: "publisher-2", TrackID: "other-video"}:       {kind: "video"},
			{PublisherID: "publisher-1", TrackID: "unknown"}:           {kind: "data"},
		},
	}

	got := publisherMediaTracks(router, "publisher-1")
	want := []serveMediaTrack{
		{ID: "audio", Kind: "audio"},
		{ID: "video", Kind: "video"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("publisherMediaTracks() = %#v, want %#v", got, want)
	}
}

func TestRemoteViewerRequestSubscriberID(t *testing.T) {
	tests := []struct {
		name string
		req  m7hRemoteViewerRequest
		want string
	}{
		{name: "defaults to viewer identity", req: m7hRemoteViewerRequest{ViewerID: " viewer-1 "}, want: "viewer-1"},
		{name: "uses unique subscriber identity", req: m7hRemoteViewerRequest{ViewerID: "viewer-1", SubscriberID: " viewer-1-streamer-1 "}, want: "viewer-1-streamer-1"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.req.subscriberID(); got != test.want {
				t.Fatalf("subscriberID() = %q, want %q", got, test.want)
			}
		})
	}
}
