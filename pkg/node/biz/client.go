package biz

import (
	"encoding/json"

	nprotoo "github.com/cloudwebrtc/nats-protoo"
	"github.com/google/uuid"
	"github.com/notedit/sdp"
	"github.com/pion/ion/pkg/log"
	"github.com/pion/ion/pkg/proto"
	"github.com/pion/ion/pkg/signal"
	"github.com/pion/ion/pkg/util"
)

var (
	ridError  = util.NewNpError(codeRoomErr, codeStr(codeRoomErr))
	jsepError = util.NewNpError(codeJsepErr, codeStr(codeJsepErr))
	// sdpError  = util.NewNpError(codeSDPErr, codeStr(codeSDPErr))
	midError = util.NewNpError(codeMIDErr, codeStr(codeMIDErr))
)

// join room
func join(peer *signal.Peer, msg proto.FromClientJoinMsg) (interface{}, *nprotoo.Error) {
	log.Infof("biz.join peer.ID()=%s msg=%v", peer.ID(), msg)
	rid := msg.RID

	// Validate
	if msg.RID == "" {
		return nil, ridError
	}
	sdpInfo, err := sdp.Parse(msg.Jsep.SDP)
	if err != nil {
		return nil, util.NewNpError(400, "Could not parse SDP")
	}

	islb := getRPCForIslb()
	if islb == nil {
		return nil, util.NewNpError(500, "Not found any node for islb.")
	}
	uid := peer.ID()

	// already joined this room, removing old peer
	if p := signal.GetPeer(rid, uid); p != nil {
		log.Infof("biz.join peer.ID()=%s already joined, removing old peer", uid)
		if _, err := islb.SyncRequest(proto.IslbPeerLeave, proto.IslbPeerLeaveMsg{
			RoomInfo: proto.RoomInfo{UID: uid, RID: msg.RID},
		}); err != nil {
			log.Errorf("IslbClientOnLeave failed %v", err.Error())
		}
		p.Close()
	}
	log.Infof("biz.join adding new peer")
	signal.AddPeer(rid, peer)

	mid := proto.MID(uuid.New().String())
	_, sfu, npErr := getRPCForNode("sfu", islb, uid, rid, mid)
	if npErr != nil {
		log.Errorf("error getting sfu: %v", npErr)
		return nil, util.NewNpError(500, "Not found any node for sfu.")
	}
	info := msg.Info

	// Send join => islb
	resp, npErr := islb.SyncRequest(proto.IslbPeerJoin, proto.ToIslbPeerJoinMsg{
		UID: uid, RID: rid, MID: mid, Info: info,
	})
	if npErr != nil {
		log.Errorf("IslbClientOnJoin failed %v", npErr)
	}
	var fromIslbPeerJoinMsg proto.FromIslbPeerJoinMsg
	if err := json.Unmarshal(resp, &fromIslbPeerJoinMsg); err != nil {
		log.Errorf("IslbClientOnJoin failed %v", err)
	}

	// Send join => sfu
	resp, npErr = sfu.SyncRequest(proto.SfuClientJoin, proto.ToSfuJoinMsg{
		UID:     uid,
		RID:     rid,
		MID:     mid,
		SID:     fromIslbPeerJoinMsg.SID,
		RTCInfo: msg.RTCInfo,
	})
	if npErr != nil {
		log.Errorf("SfuClientOnJoin failed %v", npErr)
	}
	var fromSfuJoinMsg proto.FromSfuJoinMsg
	if err := json.Unmarshal(resp, &fromSfuJoinMsg); err != nil {
		log.Errorf("SfuClientOnJoin failed %v", err)
	}

	// Associate the stream in the SDP with the UID/RID/MID.
	for key := range sdpInfo.GetStreams() {
		islb.AsyncRequest(proto.IslbStreamAdd, proto.ToIslbStreamAddMsg{
			UID: uid, RID: rid, MID: mid, StreamID: proto.StreamID(key),
		})
	}

	return proto.ToClientJoinMsg{
		Peers:   fromIslbPeerJoinMsg.Peers,
		Streams: fromIslbPeerJoinMsg.Streams,
		MID:     mid,
		RTCInfo: fromSfuJoinMsg.RTCInfo,
	}, nil
}

func leave(peer *signal.Peer, msg proto.FromClientLeaveMsg) (interface{}, *nprotoo.Error) {
	log.Infof("biz.leave msg=%v", msg)
	room := signal.GetRoom(msg.RID)
	if room == nil {
		log.Warnf("room not exits, rid=", msg.RID)
		return nil, nil
	}
	room.DelPeer(msg.UID)

	islb := getRPCForIslb()
	if islb == nil {
		log.Errorf("islb node not found")
		return nil, util.NewNpError(500, "islb node not found")
	}

	if _, err := islb.SyncRequest(proto.IslbPeerLeave, proto.IslbPeerLeaveMsg{
		RoomInfo: proto.RoomInfo{UID: msg.UID, RID: msg.RID},
	}); err != nil {
		log.Errorf("IslbPeerLeave error: %v", err.Error())
	}

	var mids []proto.MID
	if msg.MID == "" {
		var fromIslbListMids proto.FromIslbListMids
		if resp, err := islb.SyncRequest(proto.IslbListMids, proto.ToIslbListMids{
			UID: msg.UID,
			RID: msg.RID,
		}); err == nil {
			if err := json.Unmarshal(resp, &fromIslbListMids); err == nil {
				mids = fromIslbListMids.MIDs
			} else {
				log.Errorf("json.Unmarshal error: %v", err)
			}
		} else {
			log.Errorf("IslbListMids error: %v", err)
		}
	} else {
		mids = append(mids, msg.MID)
	}
	for _, mid := range mids {
		_, sfu, err := getRPCForNode("sfu", islb, msg.UID, msg.RID, mid)
		if err != nil {
			log.Errorf("Not found any sfu node: %d => %s", err.Code, err.Reason)
			continue
		}
		if _, err := sfu.SyncRequest(proto.SfuClientLeave, proto.ToSfuLeaveMsg{
			UID: msg.UID, RID: msg.RID, MID: mid,
		}); err != nil {
			log.Errorf("SfuClientLeave error %v", err.Error())
			continue
		}
	}

	return nil, nil
}

func offer(peer *signal.Peer, msg proto.ClientNegotiationMsg) (interface{}, *nprotoo.Error) {
	log.Infof("biz.offer peer.ID()=%s msg=%v", peer.ID(), msg)
	_, sfu, err := getRPCForNode("sfu", nil, peer.ID(), msg.RID, msg.MID)
	if err != nil {
		log.Warnf("Not found any sfu node, reject: %d => %s", err.Code, err.Reason)
		return nil, util.NewNpError(err.Code, err.Reason)
	}
	resp, err := sfu.SyncRequest(proto.SfuClientOffer, proto.SfuNegotiationMsg{
		UID:     peer.ID(),
		RID:     msg.RID,
		MID:     msg.MID,
		RTCInfo: proto.RTCInfo{Jsep: msg.Jsep},
	})
	if err != nil {
		log.Errorf("SfuClientOnOffer failed %v", err.Error())
		return nil, util.NewNpError(err.Code, err.Reason)
	}

	var answer proto.SfuNegotiationMsg
	if err := json.Unmarshal(resp, &answer); err != nil {
		log.Errorf("Parse answer failed %v", err.Error())
		return nil, util.NewNpError(500, err.Error())
	}

	return proto.ClientNegotiationMsg{
		RID:     answer.RID,
		MID:     answer.MID,
		RTCInfo: answer.RTCInfo,
	}, nil
}

func answer(peer *signal.Peer, msg proto.ClientNegotiationMsg) (interface{}, *nprotoo.Error) {
	log.Infof("biz.answer peer.ID()=%s msg=%v", peer.ID(), msg)

	_, sfu, err := getRPCForNode("sfu", nil, peer.ID(), msg.RID, msg.MID)
	if err != nil {
		log.Warnf("Not found any sfu node, reject: %d => %s", err.Code, err.Reason)
		return nil, util.NewNpError(err.Code, err.Reason)
	}

	if _, err := sfu.SyncRequest(proto.SfuClientAnswer, proto.SfuNegotiationMsg{
		UID:     peer.ID(),
		RID:     msg.RID,
		MID:     msg.MID,
		RTCInfo: msg.RTCInfo,
	}); err != nil {
		log.Errorf("SfuClientOnAnswer failed %v", err.Error())
		return nil, util.NewNpError(err.Code, err.Reason)
	}

	return nil, nil
}

func broadcast(peer *signal.Peer, msg proto.FromClientBroadcastMsg) (interface{}, *nprotoo.Error) {
	log.Infof("biz.broadcast peer.ID()=%s msg=%v", peer.ID(), msg)

	// Validate
	if msg.RID == "" {
		return nil, ridError
	}

	islb := getRPCForIslb()
	if islb == nil {
		return nil, util.NewNpError(500, "Not found any node for islb.")
	}
	islb.AsyncRequest(proto.IslbBroadcast, proto.IslbBroadcastMsg{
		RoomInfo: proto.RoomInfo{UID: peer.ID(), RID: msg.RID},
		Info:     msg.Info,
	})
	return emptyMap, nil
}

func trickle(peer *signal.Peer, msg proto.ClientTrickleMsg) (interface{}, *nprotoo.Error) {
	log.Infof("biz.trickle peer.ID()=%s msg=%v", peer.ID(), msg)
	// Validate
	if msg.RID == "" {
		return nil, ridError
	}

	_, sfu, err := getRPCForNode("sfu", nil, peer.ID(), msg.RID, msg.MID)
	if err != nil {
		log.Warnf("Not found any sfu node, reject: %d => %s", err.Code, err.Reason)
		return nil, util.NewNpError(err.Code, err.Reason)
	}

	sfu.AsyncRequest(proto.ClientTrickleICE, proto.SfuTrickleMsg{
		UID:       peer.ID(),
		RID:       msg.RID,
		MID:       msg.MID,
		Candidate: msg.Candidate,
	})
	return emptyMap, nil
}
