// Package outputs implements hami-media's own HLS/CMAF packager
// (job 13.1/13.2) as a separate process from the forwarder (13.8).
// It builds fMP4 segments (ftyp + moov init segment, moof + mdat
// media segments) and HLS playlists from Annex-B H.264 frames.
//
// This is hami-media's OWN packager: MediaMTX results do not count
// for 13.1/13.2. Safari playback cannot be verified in this VM (no
// Safari, no browser); verification here is ffprobe/ffmpeg playback
// of the produced playlist plus a segment parser check. That gap is
// recorded in docs/cluster-d/13-packager.md.
package outputs

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
)

// H264Frame is one video frame: Annex-B NAL units (with start codes)
// plus whether it is a keyframe (contains an IDR NAL, type 5).
type H264Frame struct {
	NALs      [][]byte // each NAL without its start code
	KeyFrame  bool
	Timestamp uint32 // RTP timestamp, 90 kHz clock
	Duration  uint32 // frame duration in 90 kHz ticks
}

// NALType returns the H.264 NAL unit type (low 5 bits of first byte).
func NALType(nal []byte) int {
	if len(nal) == 0 {
		return -1
	}
	return int(nal[0] & 0x1f)
}

// IsKeyFrameNAL reports whether a NAL unit is an IDR slice (type 5).
func IsKeyFrameNAL(nal []byte) bool { return NALType(nal) == 5 }

// IsParameterSet reports SPS (7) or PPS (8).
func IsParameterSet(nal []byte) bool {
	t := NALType(nal)
	return t == 7 || t == 8
}

func box(typ string, payload []byte) []byte {
	out := make([]byte, 8+len(payload))
	binary.BigEndian.PutUint32(out[0:4], uint32(len(out)))
	copy(out[4:8], typ)
	copy(out[8:], payload)
	return out
}

func fullBox(typ string, version byte, flags uint32, payload []byte) []byte {
	p := make([]byte, 4+len(payload))
	p[0] = version
	p[1] = byte(flags >> 16)
	p[2] = byte(flags >> 8)
	p[3] = byte(flags)
	copy(p[4:], payload)
	return box(typ, p)
}

// InitSegment builds the fMP4 init segment: ftyp + moov (mvhd, trak
// with tkhd/mdhd/hdlr/minf, stbl with avcC from the SPS/PPS).
func InitSegment(sps, pps []byte, width, height uint16, timescale uint32) ([]byte, error) {
	if len(sps) < 4 || len(pps) == 0 {
		return nil, errors.New("packager: valid SPS (>= 4 bytes) and PPS are required for the init segment")
	}
	if len(sps) > 0xffff || len(pps) > 0xffff {
		return nil, errors.New("packager: SPS/PPS exceeds 65535 bytes")
	}
	if timescale == 0 {
		timescale = 90000
	}
	ftyp := box("ftyp", []byte("iso6\x00\x00\x00\x00iso6mp41"))
	avcC := buildAVCC(sps, pps)
	avc1 := buildAVC1(avcC, width, height)
	// stsd: entry_count (4) + one avc1 entry. (fullBox adds the
	// version/flags; the payload must NOT include a second copy.)
	stsdPayload := make([]byte, 4)
	binary.BigEndian.PutUint32(stsdPayload[0:4], 1)
	stsd := fullBox("stsd", 0, 0, append(stsdPayload, avc1...))
	stts := fullBox("stts", 0, 0, make([]byte, 4)) // 0 entries; duration via tfdt
	stsc := fullBox("stsc", 0, 0, make([]byte, 4))
	stsz := fullBox("stsz", 0, 0, make([]byte, 8))
	stco := fullBox("stco", 0, 0, make([]byte, 4))
	stbl := box("stbl", concat(stsd, stts, stsc, stsz, stco))
	dref := fullBox("dref", 0, 0, concat(uint32Bytes(1), box("url ", []byte{0, 0, 0, 1})))
	dinf := box("dinf", dref)
	vmhd := fullBox("vmhd", 0, 1, make([]byte, 8))
	minf := box("minf", concat(vmhd, dinf, stbl))
	mdhd := buildMDHD(timescale)
	hdlr := buildHDLR("vide")
	mdia := box("mdia", concat(mdhd, hdlr, minf))
	tkhd := buildTKHD(width, height)
	trak := box("trak", concat(tkhd, mdia))
	mvhd := buildMVHD(timescale)
	// mvex/trex marks the track as fragmented: without it, players
	// (ffmpeg included) do not expect moof boxes for the track.
	trexPayload := make([]byte, 20)
	binary.BigEndian.PutUint32(trexPayload[0:4], 1) // track id
	binary.BigEndian.PutUint32(trexPayload[4:8], 1) // default sample description index
	trex := fullBox("trex", 0, 0, trexPayload)
	mvex := box("mvex", trex)
	moov := box("moov", concat(mvhd, trak, mvex))
	return concat(ftyp, moov), nil
}

func buildAVCC(sps, pps []byte) []byte {
	// AVCDecoderConfigurationRecord.
	out := []byte{1, sps[1], sps[2], sps[3], 0xff, 0xe1}
	out = append(out, byte(len(sps)>>8), byte(len(sps)))
	out = append(out, sps...)
	out = append(out, 1, byte(len(pps)>>8), byte(len(pps)))
	out = append(out, pps...)
	return out
}

func buildAVC1(avcC []byte, width, height uint16) []byte {
	// VisualSampleEntry: 6 reserved + data ref idx, then pre-defined
	// fields, width, height, resolutions, frame count, compressor
	// name (32 bytes), depth, pre-defined.
	p := make([]byte, 78)
	binary.BigEndian.PutUint16(p[6:8], 1) // data_reference_index
	binary.BigEndian.PutUint16(p[24:26], width)
	binary.BigEndian.PutUint16(p[26:28], height)
	binary.BigEndian.PutUint32(p[28:32], 0x00480000) // hresolution 72dpi
	binary.BigEndian.PutUint32(p[32:36], 0x00480000)
	binary.BigEndian.PutUint16(p[40:42], 1)      // frame_count
	binary.BigEndian.PutUint16(p[74:76], 0x0018) // depth 24
	binary.BigEndian.PutUint16(p[76:78], 0xffff)
	entry := box("avc1", concat(p, box("avcC", avcC)))
	return entry
}

func buildMVHD(timescale uint32) []byte {
	// mvhd v0 payload (88 bytes, after version/flags): creation(4),
	// modification(4), timescale(4), duration(4), rate(4),
	// volume(2), reserved(2), matrix(36), pre_defined(24),
	// next_track_id(4).
	p := make([]byte, 88)
	binary.BigEndian.PutUint32(p[8:12], timescale)
	binary.BigEndian.PutUint32(p[12:16], 0)          // duration unknown
	binary.BigEndian.PutUint32(p[16:20], 0x00010000) // rate 1.0
	binary.BigEndian.PutUint16(p[20:22], 0x0100)     // volume
	// unity matrix: a at 24, d at 40, w at 56 (payload offsets).
	binary.BigEndian.PutUint32(p[24:28], 0x00010000)
	binary.BigEndian.PutUint32(p[40:44], 0x00010000)
	binary.BigEndian.PutUint32(p[56:60], 0x40000000)
	binary.BigEndian.PutUint32(p[84:88], 2) // next_track_id
	return fullBox("mvhd", 0, 0, p)
}

func buildTKHD(width, height uint16) []byte {
	// tkhd v0 payload (80 bytes, after version/flags): creation(4),
	// modification(4), track_id(4), reserved(4), duration(4),
	// reserved(8), layer(2), alternate_group(2), volume(2),
	// reserved(2), matrix(36), width(4), height(4).
	p := make([]byte, 80)
	binary.BigEndian.PutUint32(p[8:12], 1)  // track id
	binary.BigEndian.PutUint32(p[16:20], 0) // duration unknown
	binary.BigEndian.PutUint32(p[36:40], 0x00010000)
	binary.BigEndian.PutUint32(p[52:56], 0x00010000)
	binary.BigEndian.PutUint32(p[68:72], 0x40000000)
	binary.BigEndian.PutUint32(p[72:76], uint32(width)<<16)
	binary.BigEndian.PutUint32(p[76:80], uint32(height)<<16)
	return fullBox("tkhd", 0, 7, p)
}

func buildMDHD(timescale uint32) []byte {
	// mdhd v0 payload (20 bytes): creation(4), modification(4),
	// timescale(4), duration(4), language(2), pre_defined(2).
	p := make([]byte, 20)
	binary.BigEndian.PutUint32(p[8:12], timescale)
	binary.BigEndian.PutUint32(p[12:16], 0)      // duration unknown
	binary.BigEndian.PutUint16(p[16:18], 0x55c4) // und
	return fullBox("mdhd", 0, 0, p)
}

func buildHDLR(kind string) []byte {
	// hdlr payload: pre_defined(4), handler_type(4), reserved(12), name.
	p := make([]byte, 32)
	copy(p[4:8], kind)
	copy(p[20:32], "VideoHandler\x00")
	return fullBox("hdlr", 0, 0, p)
}

func concat(parts ...[]byte) []byte {
	var out []byte
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

func uint32Bytes(v uint32) []byte {
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, v)
	return b
}

// MediaSegment builds one fMP4 media segment (moof + mdat) for a
// group of frames. The first frame MUST be a keyframe (13.2); the
// packager refuses a segment that does not start on one.
func MediaSegment(frames []H264Frame, sequenceNumber uint32, baseDecodeTime uint64) ([]byte, error) {
	if len(frames) == 0 {
		return nil, errors.New("packager: segment needs at least one frame")
	}
	if !frames[0].KeyFrame {
		return nil, errors.New("packager: segment must start on a keyframe (13.2)")
	}
	var mdatPayload bytes.Buffer
	var totalDuration uint32
	var sampleSizes []uint32
	for _, fr := range frames {
		var sample bytes.Buffer
		for _, nal := range fr.NALs {
			var lenBuf [4]byte
			binary.BigEndian.PutUint32(lenBuf[:], uint32(len(nal)))
			sample.Write(lenBuf[:])
			sample.Write(nal)
		}
		sampleSizes = append(sampleSizes, uint32(sample.Len()))
		mdatPayload.Write(sample.Bytes())
		totalDuration += fr.Duration
	}
	// trun: data-offset-present + sample-duration + sample-size.
	// Layout after version/flags: sample_count(4), data_offset(4),
	// then per-sample duration+size.
	trunBody := make([]byte, 8+len(frames)*8)
	binary.BigEndian.PutUint32(trunBody[0:4], uint32(len(frames)))
	for i, fr := range frames {
		binary.BigEndian.PutUint32(trunBody[8+i*8:12+i*8], fr.Duration)
		binary.BigEndian.PutUint32(trunBody[12+i*8:16+i*8], sampleSizes[i])
	}
	trun := fullBox("trun", 1, 0x000001|0x000100|0x000200, trunBody)
	tfhdPayload := make([]byte, 4)
	binary.BigEndian.PutUint32(tfhdPayload[0:4], 1)   // track id
	tfhd := fullBox("tfhd", 0, 0x020000, tfhdPayload) // default-base-is-moof
	tfdtPayload := make([]byte, 8)
	binary.BigEndian.PutUint64(tfdtPayload, baseDecodeTime)
	tfdt := fullBox("tfdt", 1, 0, tfdtPayload)
	traf := box("traf", concat(tfhd, tfdt, trun))
	mfhdPayload := uint32Bytes(sequenceNumber)
	mfhd := fullBox("mfhd", 0, 0, mfhdPayload)
	moof := box("moof", concat(mfhd, traf))
	// Patch the trun data-offset: it is relative to the start of the
	// enclosing moof box and points at the first sample byte, which
	// is past the mdat box header (moof size + 8). ffmpeg's own
	// fMP4 output confirms this (dataoff = moofSize + 8).
	dataOffset := uint32(len(moof) + 8)
	// trun box layout: size(4) type(4) version/flags(4) sample-count(4)
	// data-offset(4) ... locate it: moof contains mfhd then traf;
	// recompute offset of the data-offset field inside moof.
	// moof = box(mfhd(16) + traf(...)); traf = box(tfhd(16)+tfdt(20)+trun).
	// trun data-offset field is at: 8(moof hdr)+16(mfhd)+8(traf hdr)+16(tfhd)+20(tfdt)+8(trun hdr)+4(version/flags)+4(sample-count) = 84.
	const trunDataOffsetPos = 8 + 16 + 8 + 16 + 20 + 8 + 4 + 4
	binary.BigEndian.PutUint32(moof[trunDataOffsetPos:trunDataOffsetPos+4], dataOffset)
	mdat := box("mdat", mdatPayload.Bytes())
	return concat(moof, mdat), nil
}

// SegmentInfo describes one packaged segment for the playlist.
type SegmentInfo struct {
	Filename     string
	DurationSecs float64
	KeyFrame     bool // first frame is a keyframe
	Frames       int
}

// MediaPlaylist renders an HLS media playlist (EXT-X-VERSION 7,
// fMP4, EXT-X-MAP to the init segment).
func MediaPlaylist(segments []SegmentInfo, initFilename string, targetDuration float64) string {
	var b bytes.Buffer
	b.WriteString("#EXTM3U\n#EXT-X-VERSION:7\n")
	fmt.Fprintf(&b, "#EXT-X-TARGETDURATION:%d\n", int(targetDuration+0.999))
	b.WriteString("#EXT-X-MEDIA-SEQUENCE:0\n")
	fmt.Fprintf(&b, "#EXT-X-MAP:URI=%q\n", initFilename)
	for _, s := range segments {
		fmt.Fprintf(&b, "#EXTINF:%.3f,\n%s\n", s.DurationSecs, s.Filename)
	}
	b.WriteString("#EXT-X-ENDLIST\n")
	return b.String()
}

// MasterPlaylist renders a one-rendition master playlist.
func MasterPlaylist(mediaPlaylist string, bandwidth int, width, height int) string {
	return fmt.Sprintf("#EXTM3U\n#EXT-X-VERSION:7\n#EXT-X-STREAM-INF:BANDWIDTH=%d,RESOLUTION=%dx%d,CODECS=\"avc1.64001f\"\n%s\n",
		bandwidth, width, height, mediaPlaylist)
}

// ParseAnnexB splits Annex-B H.264 into frames. Each slice NAL
// (type 1 or 5) starts a new frame; SPS/PPS/AUD NALs attach to the
// following frame. Frame duration defaults to 3000 ticks at 90 kHz
// (30 fps); callers with a known frame rate should rescale
// Duration afterwards. Returns the frames plus the first SPS/PPS.
func ParseAnnexB(raw []byte) (frames []H264Frame, sps, pps []byte, err error) {
	nals := splitAnnexB(raw)
	if len(nals) == 0 {
		return nil, nil, nil, errors.New("packager: no NAL units found")
	}
	var pending [][]byte
	var pendingKey bool
	flush := func() {
		if len(pending) == 0 {
			return
		}
		frames = append(frames, H264Frame{NALs: pending, KeyFrame: pendingKey, Duration: 3000})
		pending = nil
		pendingKey = false
	}
	for _, nal := range nals {
		t := NALType(nal)
		switch t {
		case 7:
			if sps == nil {
				sps = append([]byte(nil), nal...)
			}
			pending = append(pending, nal)
		case 8:
			if pps == nil {
				pps = append([]byte(nil), nal...)
			}
			pending = append(pending, nal)
		case 9: // AUD: starts a new access unit
			flush()
		case 1, 5:
			if len(pending) > 0 {
				// A slice after parameter sets starts a new frame
				// only if the pending group already has a slice.
				hasSlice := false
				for _, p := range pending {
					if NALType(p) == 1 || NALType(p) == 5 {
						hasSlice = true
					}
				}
				if hasSlice {
					flush()
				}
			}
			pending = append(pending, nal)
			if t == 5 {
				pendingKey = true
			}
		default:
			pending = append(pending, nal)
		}
	}
	flush()
	if len(frames) == 0 {
		return nil, nil, nil, errors.New("packager: no frames found")
	}
	if sps == nil || pps == nil {
		return nil, nil, nil, errors.New("packager: stream has no SPS/PPS")
	}
	for i := range frames {
		frames[i].Timestamp = uint32(i * 3000)
	}
	return frames, sps, pps, nil
}

// splitAnnexB splits Annex-B bytes into NAL units (start codes removed).
func splitAnnexB(raw []byte) [][]byte {
	var nals [][]byte
	start := -1
	for i := 0; i+3 < len(raw); i++ {
		isStart := false
		skip := 0
		if raw[i] == 0 && raw[i+1] == 0 && raw[i+2] == 1 {
			isStart, skip = true, 3
		} else if i+4 < len(raw) && raw[i] == 0 && raw[i+1] == 0 && raw[i+2] == 0 && raw[i+3] == 1 {
			isStart, skip = true, 4
		}
		if !isStart {
			continue
		}
		if start >= 0 {
			nals = append(nals, raw[start:i])
		}
		start = i + skip
		i += skip - 1
	}
	if start >= 0 && start < len(raw) {
		nals = append(nals, raw[start:])
	}
	return nals
}

// Packager accumulates frames into keyframe-aligned segments.
type Packager struct {
	framesPerSegment int
	pending          []H264Frame
	segments         [][]H264Frame
	baseTime         uint64
}

// NewPackager groups framesPerSegment frames per segment. A segment
// is only closed on a keyframe boundary, so segments may be longer
// than framesPerSegment when keyframes are sparse.
func NewPackager(framesPerSegment int) (*Packager, error) {
	if framesPerSegment <= 0 {
		return nil, errors.New("packager: framesPerSegment must be > 0")
	}
	return &Packager{framesPerSegment: framesPerSegment}, nil
}

// Add appends a frame; when enough frames have accumulated AND the
// next frame is a keyframe, the pending segment closes.
func (p *Packager) Add(fr H264Frame) {
	p.pending = append(p.pending, fr)
}

// Close flushes the pending segment. Segments always start on a
// keyframe: if the first pending frame is not a keyframe, Close
// drops frames up to the first keyframe and reports how many were
// dropped (they cannot start a segment, 13.2).
func (p *Packager) Close() (segments [][]H264Frame, dropped int) {
	pending := p.pending
	p.pending = nil
	for len(pending) > 0 && !pending[0].KeyFrame {
		pending = pending[1:]
		dropped++
	}
	// Split into keyframe-aligned chunks of ~framesPerSegment.
	for len(pending) > 0 {
		end := p.framesPerSegment
		if end > len(pending) {
			end = len(pending)
		}
		// Extend to the next keyframe boundary if one exists inside.
		for i := end; i < len(pending); i++ {
			if pending[i].KeyFrame {
				end = i
				break
			}
		}
		// If no keyframe boundary found before the end, take what we have.
		if end == 0 {
			end = len(pending)
		}
		p.segments = append(p.segments, pending[:end])
		pending = pending[end:]
		// Next chunk must start on a keyframe.
		for len(pending) > 0 && !pending[0].KeyFrame {
			pending = pending[1:]
			dropped++
		}
	}
	return p.segments, dropped
}
