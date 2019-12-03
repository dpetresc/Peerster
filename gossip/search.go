package gossip

import (
	"encoding/hex"
	"fmt"
	"github.com/dpetresc/Peerster/util"
	"math/rand"
	"net"
	"regexp"
	"strings"
	"sync"
	"time"
)

var fullMatchThreshold uint64 = 2
var maxBudget uint64 = 32

type searchRequestIdentifier struct {
	Origin string
	Keywords string
}

type LockRecentSearchRequest struct {
	// keep track of the recent search requests
	Requests map[searchRequestIdentifier]bool
	sync.RWMutex
}

type FileSearchIdentifier struct {
	Filename string
	Metahash string
}

type MatchStatus struct {
	chunksDistribution map[uint64][]string
	totalNbChunk uint64
}

type LockSearchMatches struct {
	currNbFullMatch uint64
	Matches map[FileSearchIdentifier]*MatchStatus
	sync.RWMutex
}

func (gossiper *Gossiper) removeSearchReqestWhenTimeout(searchRequestId searchRequestIdentifier) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	select {
	case <-ticker.C:
		gossiper.lRecentSearchRequest.Lock()
		delete(gossiper.lRecentSearchRequest.Requests, searchRequestId)
		gossiper.lRecentSearchRequest.Unlock()
	}
}

func (gossiper *Gossiper) handleSearchRequestPacket(packet *util.GossipPacket, sourceAddr *net.UDPAddr) {
	// check equality in less than 0.5 seconds
	var sourceAddrString string
	if sourceAddr != nil {
		sourceAddrString = util.UDPAddrToString(sourceAddr)
	}else {
		sourceAddrString = ""
	}
	searchRequestId := searchRequestIdentifier{
		Origin:   packet.SearchRequest.Origin,
		Keywords: strings.Join(packet.SearchRequest.Keywords, ","),
	}
	gossiper.lRecentSearchRequest.Lock()
	if _, ok := gossiper.lRecentSearchRequest.Requests[searchRequestId]; !ok {
		gossiper.lRecentSearchRequest.Requests[searchRequestId] = true
		gossiper.lRecentSearchRequest.Unlock()
		if packet.SearchRequest.Origin != gossiper.Name {
			results := gossiper.searchFiles(packet.SearchRequest.Keywords)
			if len(results) > 0 {
				searchReply := util.SearchReply{
					Origin:      gossiper.Name,
					Destination: packet.SearchRequest.Origin,
					HopLimit:    util.HopLimit,
					Results:     results,
				}
				gossiper.handleSearchReplyPacket(&util.GossipPacket{
					SearchReply:   &searchReply,
				})
			}
		}
		gossiper.redistributeSearchRequest(packet, sourceAddrString)
		go gossiper.removeSearchReqestWhenTimeout(searchRequestId)
	} else {
		gossiper.lRecentSearchRequest.Unlock()
	}
}

func createRegexp(keywords []string) *regexp.Regexp{
	var expr string = ""
	for _, keyword := range keywords[:len(keywords)-1] {
		expr += fmt.Sprintf(".*%s.*|", keyword)
	}
	expr += keywords[len(keywords)-1]
	regex, err := regexp.Compile(expr)
	util.CheckError(err)
	return regex
}

func (gossiper *Gossiper) addMatchingFile(metadata *MyFile,
	matchingfile []*util.SearchResult) []*util.SearchResult {
	metahash, err := hex.DecodeString(metadata.metahash)
	util.CheckError(err)
	fmt.Println(metadata.nbChunks)
	chunkMap := make([]uint64, 0, metadata.nbChunks)
	for i, _ := range metadata.Metafile {
		chunkMap = append(chunkMap, uint64(i+1))
	}
	result := util.SearchResult{
		FileName:     metadata.fileName,
		MetafileHash: metahash,
		ChunkMap:     chunkMap,
		ChunkCount:   uint64(len(metadata.Metafile)),
	}
	matchingfile = append(matchingfile, &result)
	return matchingfile
}

func (gossiper *Gossiper) searchFiles(keywords []string) []*util.SearchResult{
	regex := createRegexp(keywords)
	matchingfile := make([]*util.SearchResult, 0)
	matchingfileMap := make(map[FileSearchIdentifier]bool)
	gossiper.lFiles.RLock()
	// completed downloads
	for file := range gossiper.lFiles.Files {
		if matched := regex.MatchString(file); matched {
			metadata := gossiper.lFiles.Files[file]

			fSId := FileSearchIdentifier{
				Filename: metadata.fileName,
				Metahash: metadata.metahash,
			}
			matchingfileMap[fSId] = true
			matchingfile = gossiper.addMatchingFile(metadata, matchingfile)
		}
	}
	gossiper.lFiles.RUnlock()

	gossiper.lUncompletedFiles.RLock()
	for file := range gossiper.lUncompletedFiles.IncompleteFiles {
		if matched := regex.MatchString(file); matched {
			files := gossiper.lUncompletedFiles.IncompleteFiles[file]
			for downloadId := range files {
				metadata := gossiper.lUncompletedFiles.IncompleteFiles[file][downloadId]

				fSId := FileSearchIdentifier{
					Filename: metadata.fileName,
					Metahash: metadata.metahash,
				}
				if _, ok := matchingfileMap[fSId]; ok {
					// already have complete file no need to add incomplete result
					continue
				} else {
					matchingfile = gossiper.addMatchingFile(metadata, matchingfile)
				}
			}
		}
	}
	gossiper.lUncompletedFiles.RUnlock()

	return matchingfile
}

func (gossiper *Gossiper) createPacketsToDistribute(budgetToDistribute uint64, nbNeighbors uint64, packet *util.GossipPacket) (uint64, *util.GossipPacket, *util.GossipPacket) {
	baseBudget := budgetToDistribute / nbNeighbors
	surplusBudget := budgetToDistribute % nbNeighbors
	var packetSurplus *util.GossipPacket = nil
	var packetBase *util.GossipPacket = nil
	if surplusBudget > 0 {
		packetSurplus = &util.GossipPacket{SearchRequest: &util.SearchRequest{
			Origin:   packet.SearchRequest.Origin,
			Budget:   baseBudget + 1,
			Keywords: packet.SearchRequest.Keywords,
		}}
	}
	if baseBudget != 0 {
		packetBase = &util.GossipPacket{SearchRequest: &util.SearchRequest{
			Origin:   packet.SearchRequest.Origin,
			Budget:   baseBudget,
			Keywords: packet.SearchRequest.Keywords,
		}}
	}
	return surplusBudget, packetSurplus, packetBase
}

func (gossiper *Gossiper) redistributeSearchRequest(packet *util.GossipPacket, sourceAddr string) {
	var neighbors []string
	var nbNeighbors uint64 = 0
	gossiper.Peers.RLock()
	for neighbor := range gossiper.Peers.PeersMap {
		if neighbor != sourceAddr{
			neighbors = append(neighbors, neighbor)
			nbNeighbors += 1
		}
	}
	gossiper.Peers.RUnlock()

	rand.Seed(time.Now().UnixNano())
	rand.Shuffle(int(nbNeighbors), func(i, j int) { neighbors[i], neighbors[j] = neighbors[j], neighbors[i] })

	budgetToDistribute := packet.SearchRequest.Budget - 1
	if budgetToDistribute > 0 {
		if nbNeighbors > 0 {
			surplusNeighbor, packetSurplus, packetBase := gossiper.createPacketsToDistribute(budgetToDistribute, nbNeighbors, packet)
			for _, neighbor := range neighbors {
				if surplusNeighbor == 0 {
					if packetBase == nil {
						return
					}
					gossiper.sendPacketToPeer(neighbor, packetBase)
				} else {
					gossiper.sendPacketToPeer(neighbor, packetSurplus)
					surplusNeighbor = surplusNeighbor - 1
				}
			}
		}
	}
}
