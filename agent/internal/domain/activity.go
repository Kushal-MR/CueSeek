package domain

// Activity: what a service is *doing*, as distinct from whether it is up.
//
// Two capabilities live here, and both are specified semantically rather than around the
// service that happened to implement them first. ADR-0005 makes capability names public
// API — `now_playing` will be shared by Plex and Emby, `transfers` by SABnzbd and Sonarr —
// so a shape borrowed from Jellyfin's or qBittorrent's DTOs would need a `_v2` the moment
// a second implementation arrived.
//
// Both carry **counts and a bounded sample**. The counts are the truth; `Items` is what a
// screen can show. A host with four hundred torrents must not put four hundred objects on
// an SSE frame every thirty seconds, and a client that displayed `len(Items)` as the total
// would quietly lie once the cap bit.

// MaxActivityItems bounds every sampled list.
//
// Twenty. It was ten while the detail sheet could not scroll, on the reasoning that nobody
// looks past the fold — true at the time, and wrong once the sheet scrolled. Twenty covers
// an ordinary home library almost entirely at roughly three kilobytes per service per
// frame, which is nothing on a tailnet.
//
// The cap is the agent's, not the client's: bandwidth is the scarce resource, and trimming
// after transmission would save nothing. It is also not a substitute for ordering — a
// sample is only useful if the *right* items are in it, which is why adapters sort before
// they trim.
const MaxActivityItems = 20

// NowPlaying is the active playback on a media server.
type NowPlaying struct {
	// Sessions is the true total, which may exceed len(Items).
	Sessions int

	// Transcoding is how many of those the server is re-encoding.
	//
	// Broken out because it is the operationally significant number on a self-hosted
	// box. Direct play costs a media server almost nothing; one 4K transcode can
	// saturate the CPU that Jellyfin, qBittorrent and everything else are sharing. A
	// console that reported only a session count would hide the fact that explains why
	// the machine is hot and why everything else just got slower.
	Transcoding int

	Items []PlaybackSession
}

// PlaybackSession is one thing being played.
type PlaybackSession struct {
	ID    string
	Title string

	// Subtitle carries context the title alone does not — an episode, an artist, a year.
	// Empty when the service offers none, and never synthesised: inventing "Season 2"
	// from a path would be the agent guessing at a domain it does not own.
	Subtitle string

	User   string
	Client string

	// PositionSeconds and DurationSeconds are zero when unknown. Duration is legitimately
	// zero for live content, which is why neither is a pointer: absent and zero mean the
	// same thing to a progress bar, and a pointer would imply a distinction that is not
	// there.
	PositionSeconds int
	DurationSeconds int

	Paused      bool
	Transcoding bool
}

// Transfers is in-flight work on a download client.
type Transfers struct {
	// Active is transfers currently moving data.
	Active int

	// Total is everything the service tracks, including paused, queued, seeding and
	// finished. Always at least Active.
	//
	// Both are kept because they answer different questions. "Is anything happening right
	// now" is Active; "how much is this thing looking after" is Total, and a client that
	// had only one of them would have to mislead about the other.
	Total int

	// Rates are the service's own aggregate, in bytes per second — never a sum over
	// Items, which is a sample and would understate them.
	DownloadRateBytes int64
	UploadRateBytes   int64

	Items []TransferItem
}

// TransferItem is one download.
type TransferItem struct {
	ID   string
	Name string

	// State is the service's own word, verbatim: `downloading`, `stalledDL`, `seeding`,
	// `queuedDL`. Deliberately not mapped onto a shared vocabulary — the difference
	// between "stalled" and "queued" is exactly what tells an operator whether to care,
	// and a lowest-common-denominator enum would discard it.
	State string

	// Progress is 0 to 1.
	Progress float32

	SizeBytes         int64
	DownloadRateBytes int64

	// ETASeconds is zero when unknown. Adapters must normalise their service's
	// "effectively never" placeholder to zero rather than passing it through — qBittorrent
	// reports 8640000, which renders as eight years and looks like a bug in CueSeek.
	ETASeconds int
}

// Bounded returns items trimmed to [MaxActivityItems].
//
// A helper rather than a rule each adapter remembers: the cap is a property of the
// contract, and an adapter that forgot it would be discovered only by a slow stream on
// somebody's phone.
func Bounded[T any](items []T) []T {
	if len(items) > MaxActivityItems {
		return items[:MaxActivityItems]
	}
	return items
}
