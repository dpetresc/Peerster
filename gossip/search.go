package gossip

import (
	"fmt"
	"github.com/dpetresc/Peerster/util"
	"regexp"
)

type searchRequestStatus struct {
	currSearchRequest string

}

func (gossiper *Gossiper) handleSearchRequestPacket(packet *util.GossipPacket, budgetSpecified bool) {
	// TODO
	fmt.Println(packet.SearchRequest.Budget)
	gossiper.searchFile(packet.SearchRequest.Keywords)
	// TODO check equality in less than 0.5 seconds
	gossiper.redistributeSearchRequest(packet)
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

func (gossiper *Gossiper) searchFile(keywords []string) {
	// TODO changer également la manière dont j'indexe les files
	// pour indexer ceux qui ne sont pas encore totalement dowloadé
	regex := createRegexp(keywords)
	matchingfile := make([]util.SearchResult, 0)
	gossiper.lFiles.RLock()
	for file := range gossiper.lFiles.Files {
		if matched := regex.MatchString(file); matched {
			// TODO
			/*metadata := gossiper.lFiles.Files[file]
			result := util.SearchResult{
				FileName:     metadata.fileName,
				MetafileHash: ,
				ChunkMap:     nil,
				ChunkCount:   0,
			}
			matchingfile = append(matchingfile, file)*/
		}
	}
	gossiper.lFiles.RUnlock()
	if len(matchingfile) > 0 {
		// TODO
	}
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

func (gossiper *Gossiper) redistributeSearchRequest(packet *util.GossipPacket) {
	budgetToDistribute := packet.SearchRequest.Budget - 1
	if budgetToDistribute > 0 {
		gossiper.Peers.RLock()
		nbNeighbors := uint64(len(gossiper.Peers.PeersMap))
		if nbNeighbors > 0 {
			surplusNeighbor, packetSurplus, packetBase := gossiper.createPacketsToDistribute(budgetToDistribute, nbNeighbors, packet)
			for neighbor := range gossiper.Peers.PeersMap {
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
		gossiper.Peers.RUnlock()
	}
}

func (gossiper *Gossiper) handleSearchReplyPacket(packet *util.GossipPacket) {
	// TODO
}
