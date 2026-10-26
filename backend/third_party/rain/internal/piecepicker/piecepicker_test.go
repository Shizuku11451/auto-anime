package piecepicker

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/cenkalti/rain/internal/bitfield"
	"github.com/cenkalti/rain/internal/peer"
	"github.com/cenkalti/rain/internal/piece"
)

func testPicker(count int) (*PiecePicker, *peer.Peer, []*peer.Peer) {
	pieces := make([]piece.Piece, count)
	for i := range pieces {
		pieces[i].Index = uint32(i)
	}
	peers := make([]*peer.Peer, 8)
	for i := range peers {
		peers[i] = &peer.Peer{Bitfield: bitfield.New(uint32(count))}
	}
	return New(pieces, 3, nil), peers[0], peers
}

func TestPickerSelection(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	for _, mode := range []string{"rarest", "endgame", "stalled"} {
		t.Run(mode, func(t *testing.T) {
			for trial := 0; trial < 300; trial++ {
				picker, target, peers := testPicker(64)
				for i := range picker.pieces {
					mp := &picker.pieces[i]
					mp.Done = rng.Intn(6) == 0
					mp.Writing = rng.Intn(6) == 0
					for _, pe := range peers {
						if rng.Intn(2) == 0 {
							picker.HandleHave(pe, uint32(i))
						}
						if pe != target && rng.Intn(3) == 0 {
							mp.Requested.Add(pe)
							switch rng.Intn(3) {
							case 0:
								mp.Snubbed.Add(pe)
							case 1:
								mp.Choked.Add(pe)
							}
						}
					}
				}
				// Check the policy independently; ties may select any equally good piece.
				best := int(^uint(0) >> 1)
				hasUnrequested := false
				for i := range picker.pieces {
					mp := &picker.pieces[i]
					if mp.Done || mp.Writing {
						continue
					}
					hasUnrequested = hasUnrequested || mp.Requested.Len() == 0
					if !mp.Having.Has(target) {
						continue
					}
					score := mp.Having.Len()
					switch mode {
					case "rarest":
						if mp.Requested.Len() != 0 {
							continue
						}
					case "endgame", "stalled":
						if mp.Requested.Len() >= picker.maxDuplicateDownload {
							continue
						}
						score = mp.RunningDownloads()
						if mode == "stalled" {
							if score != 0 {
								continue
							}
							score = mp.StalledDownloads()
						}
					}
					if score < best {
						best = score
					}
				}
				var got *myPiece
				switch mode {
				case "rarest":
					got = picker.pickRarest(target)
					if picker.endgame != !hasUnrequested {
						t.Fatal("incorrect endgame transition")
					}
				case "endgame":
					got = picker.pickEndgame(target)
				case "stalled":
					got = picker.pickStalled(target)
				}
				if got == nil {
					if best != int(^uint(0)>>1) {
						t.Fatal("missed eligible piece")
					}
					continue
				}
				if got.Done || got.Writing || !got.Having.Has(target) {
					t.Fatal("selected ineligible piece")
				}
				score := got.Having.Len()
				if mode == "rarest" {
					if got.Requested.Len() != 0 {
						t.Fatal("selected requested piece")
					}
				} else {
					if got.Requested.Len() >= picker.maxDuplicateDownload {
						t.Fatal("exceeded duplicate limit")
					}
					score = got.RunningDownloads()
					if mode == "stalled" {
						if score != 0 {
							t.Fatal("selected actively downloading piece")
						}
						score = got.StalledDownloads()
					}
				}
				if score != best {
					t.Fatalf("selected score %d, want %d", score, best)
				}
			}
		})
	}
}

func TestPickerLifecycle(t *testing.T) {
	picker, target, peers := testPicker(3)
	for i := range picker.pieces {
		picker.HandleHave(target, uint32(i))
	}
	picker.HandleHave(peers[1], 0)
	picker.HandleHave(peers[1], 1)
	got, fast := picker.PickFor(target)
	if got == nil || got.Index != 2 || fast {
		t.Fatal("did not prefer rarest piece")
	}
	picker.HandleHave(peers[2], 2)
	picker.HandleDisconnect(peers[1])
	picker.HandleCancelDownload(target, 2)
	got, _ = picker.PickFor(target)
	if got == nil || got.Index == 2 {
		t.Fatal("ignored changed availability")
	}
	picker.HandleChoke(target, got.Index)
	if picker.pieces[got.Index].RunningDownloads() != 0 {
		t.Fatal("choked download remains active")
	}
	picker.HandleUnchoke(target, got.Index)
	if picker.pieces[got.Index].RunningDownloads() != 1 {
		t.Fatal("unchoked download did not resume")
	}
	target.Downloading = true
	if got, _ := picker.PickFor(target); got != nil {
		t.Fatal("peer received concurrent pieces")
	}
	target.Downloading = false
	target.PeerChoking = true
	if got, _ := picker.PickFor(target); got != nil {
		t.Fatal("choked peer received non-fast piece")
	}
	target.ReceivedAllowedFast.Add(picker.pieces[2].Piece)
	if got, fast := picker.PickFor(target); got == nil || got.Index != 2 || !fast {
		t.Fatal("allowed-fast selection failed")
	}
	for i := range picker.pieces {
		picker.pieces[i].Done = true
	}
	if got, _ := picker.PickFor(target); got != nil {
		t.Fatal("selected completed piece")
	}
}

func TestPickerEndgameTransition(t *testing.T) {
	for _, unavailable := range []bool{false, true} {
		picker, target, peers := testPicker(2)
		picker.HandleHave(target, 0)
		picker.pieces[0].Requested.Add(peers[1])
		if !unavailable {
			picker.pieces[1].Requested.Add(peers[1])
		}
		got, _ := picker.PickFor(target)
		if picker.endgame == unavailable || (got != nil) == unavailable {
			t.Fatalf("unavailable=%v: wrong endgame selection", unavailable)
		}
	}
}

func TestPickerNoEligiblePieces(t *testing.T) {
	for _, count := range []int{0, 3} {
		picker, target, _ := testPicker(count)
		for i := range picker.pieces {
			picker.HandleHave(target, uint32(i))
			picker.pieces[i].Writing = true
		}
		if picker.pickRarest(target) != nil || !picker.endgame ||
			picker.pickEndgame(target) != nil || picker.pickStalled(target) != nil {
			t.Fatal("selected piece from empty or fully writing torrent")
		}
	}
}

func TestPickerDuplicateLimit(t *testing.T) {
	picker, target, peers := testPicker(1)
	picker.HandleHave(target, 0)
	mp := &picker.pieces[0]
	for _, pe := range peers[1 : 1+picker.maxDuplicateDownload] {
		mp.Requested.Add(pe)
		mp.Choked.Add(pe)
	}
	if picker.pickEndgame(target) != nil || picker.pickStalled(target) != nil {
		t.Fatal("selected piece already at duplicate limit")
	}
	picker.HandleUnchoke(peers[1], 0)
	mp.Requested.Remove(peers[2])
	mp.Choked.Remove(peers[2])
	if picker.pickStalled(target) != nil {
		t.Fatal("selected piece still downloading from another peer")
	}
}

func BenchmarkPicker(b *testing.B) {
	for _, count := range []int{4096, 16384} {
		for _, mode := range []string{"rarest", "endgame", "stalled"} {
			b.Run(fmt.Sprintf("%s/%d", mode, count), func(b *testing.B) {
				picker, target, peers := testPicker(count)
				for i := range picker.pieces {
					mp := &picker.pieces[i]
					mp.Having.Items = make([]*peer.Peer, 0, len(peers))
					mp.Done = i%5 == 0
					mp.Writing = i%13 == 0
					requests := i % 4
					if mode != "rarest" {
						requests = 1 + i%3
					}
					for j, pe := range peers {
						if j == 0 || j <= i%len(peers) {
							picker.HandleHave(pe, uint32(i))
						}
						if j > 0 && j <= requests {
							mp.Requested.Add(pe)
							if mode == "stalled" {
								mp.Choked.Add(pe)
							}
						}
					}
				}
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					var got *myPiece
					switch mode {
					case "rarest":
						got = picker.pickRarest(target)
					case "endgame":
						got = picker.pickEndgame(target)
					case "stalled":
						got = picker.pickStalled(target)
					}
					if got == nil {
						b.Fatal("no eligible piece")
					}
					mp := &picker.pieces[i%count]
					pe := peers[1+i%(len(peers)-1)]
					if !mp.Having.Remove(pe) {
						mp.Having.Add(pe)
					}
				}
			})
		}
	}
}
