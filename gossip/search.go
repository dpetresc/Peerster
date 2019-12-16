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

// GUI
//match represents a match
type match struct {
	FileName string
	MetaHash string
}
//matchInfo is an inner struct used to gather the information received by the other peers during a search.
//chunkCount uint64 is the total number of chunk for this file
//chunkMap   map[uint64]string is a mapping of the index of a chunk to the peers who owns it.
type matchInfo struct {
	chunkCount uint64
	chunkMap   map[uint64][]string
}
//Matches records all the matches of the current research and all past full matches
//All operations of Matches are thread-safe.
//matches map[match]matchInfo is the mapping of a match (filename, metahash) to the information received from the other peers.
type Matches struct {
	sync.RWMutex
	matches map[match]matchInfo
	Queue   []struct {
		FileName string
		Origin   string
		MetaHash string
	}
}
func MatchesFactory() *Matches {
	return &Matches{
		RWMutex: sync.RWMutex{},
		matches: make(map[match]matchInfo),
		Queue: make([]struct {
			FileName string
			Origin   string
			MetaHash string
		}, 0, 2),
	}
}
//Clear removes all matches that are not full matches
func (ms *Matches) Clear() {
	ms.Lock()
	defer ms.Unlock()

	ms.matches = make(map[match]matchInfo)
}

//AddNewResult adds a new result from a given origin to the matches
//It returns whether or not this is a new result
func (ms *Matches) AddNewResult(result *util.SearchResult, origin string) bool {
	ms.Lock()
	defer ms.Unlock()

	m := match{
		FileName: result.FileName,
		MetaHash: hex.EncodeToString(result.MetafileHash),
	}

	if _, ok := ms.matches[m]; !ok {
		ms.matches[m] = matchInfo{
			chunkCount: result.ChunkCount,
			chunkMap:   make(map[uint64][]string),
		}
	}

	return ms.matches[m].addInfo(result, origin)
}

func isOwner(owner string, owners []string) bool {
	for _, ow := range owners {
		if ow == owner {
			return true
		}
	}

	return false
}

//addInfo is an helper function that adds the result to the matchInfo
//It returns whether or not this is a new result
func (mi matchInfo) addInfo(result *util.SearchResult, origin string) bool {

	newResult := false

	for _, index := range result.ChunkMap {
		if owners, ok := mi.chunkMap[index]; !ok {
			mi.chunkMap[index] = make([]string, 0)
			mi.chunkMap[index] = append(mi.chunkMap[index], origin)
			newResult = true
		} else if !isOwner(origin, owners) {
			newResult = true
			mi.chunkMap[index] = append(mi.chunkMap[index], origin)
		}

	}
	return newResult
}
// END gui

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
