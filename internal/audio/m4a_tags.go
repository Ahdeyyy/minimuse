package audio

import (
	"encoding/binary"
	"io"
)

// M4ATags reads iTunes metadata without reading mdat or allocating for unknown
// atoms. Bounds apply to both traversal work and individual metadata values.
// Set artworkLimit to zero when only text tags are needed.
func M4ATags(r io.ReadSeeker, artworkLimit int) map[string][]byte {
	tags := make(map[string][]byte)
	end, err := r.Seek(0, io.SeekEnd)
	if err != nil {
		return tags
	}
	budget := 10000
	var walk func(int64, int64, int, string)
	walk = func(start, end int64, depth int, parent string) {
		if depth > 8 {
			return
		}
		for offset := start; offset <= end-8 && budget > 0; {
			budget--
			if _, err := r.Seek(offset, io.SeekStart); err != nil {
				return
			}
			var h [16]byte
			if _, err := io.ReadFull(r, h[:8]); err != nil {
				return
			}
			size, header := int64(binary.BigEndian.Uint32(h[:4])), int64(8)
			if size == 1 {
				if _, err := io.ReadFull(r, h[8:]); err != nil {
					return
				}
				u := binary.BigEndian.Uint64(h[8:])
				if u > uint64(end-offset) {
					return
				}
				size, header = int64(u), 16
			} else if size == 0 {
				size = end - offset
			}
			if size < header || size > end-offset {
				return
			}
			name := string(h[4:8])
			body, next := offset+header, offset+size
			switch name {
			case "moov", "udta", "ilst":
				walk(body, next, depth+1, name)
			case "meta":
				walk(body+4, next, depth+1, name)
			case "\xa9nam", "\xa9ART", "\xa9alb", "aART", "trkn", "disk", "covr":
				if parent == "ilst" {
					walk(body, next, depth+1, name)
				}
			case "data":
				limit := 16384
				if parent == "covr" {
					limit = artworkLimit
				}
				switch parent {
				case "\xa9nam", "\xa9ART", "\xa9alb", "aART", "trkn", "disk", "covr":
					n := next - body - 8 // type/locale precede the value
					if n > 0 && n <= int64(limit) && tags[parent] == nil {
						if _, err := r.Seek(body+8, io.SeekStart); err != nil {
							return
						}
						b := make([]byte, int(n))
						if _, err := io.ReadFull(r, b); err == nil {
							tags[parent] = b
						}
					}
				}
			}
			offset = next
		}
	}
	walk(0, end, 0, "")
	return tags
}
