package gossip

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/dpetresc/Peerster/util"
	"sync"
	"time"
)

type lockDownloadingChunks struct {
	currentDownloadingChunks map[string]chan util.DataReply
	// Pas vrmt besoin de mutex nn ?
	mutex sync.Mutex
}

type fileCurrentDownloadingStatus struct {
	currentChunkNumber uint32
	chunkHashes [][]byte
	// TODO pas plus simple de mettre le channel ici plutôt vu que de toute manière c'est en séquentiel le download
}

type lockDownload struct {
	currentDownloads map[string]*fileCurrentDownloadingStatus
	mutex sync.RWMutex
}

func checkIntegrity(hash string, data []byte) bool {
	sha := sha256.Sum256(data[:])
	hashToTest := hex.EncodeToString(sha[:])
	return hashToTest == hash
}

func (gossiper *Gossiper) removeDownloadingChanel(metahash string, channel chan util.DataReply) {
	close(channel)
	gossiper.lDownloadingChunk.mutex.Lock()
	delete(gossiper.lDownloadingChunk.currentDownloadingChunks, metahash)
	gossiper.lDownloadingChunk.mutex.Unlock()
}

func (gossiper *Gossiper) startDownload(packet *util.Message){
	metahash := hex.EncodeToString((*packet.Request)[:])

	// TODO check indexed files + should we index the files downloaded ?

	gossiper.lDownloads.mutex.RLock()
	_, ok := gossiper.lDownloads.currentDownloads[metahash]
	gossiper.lDownloads.mutex.RUnlock()
	if ok {
		fmt.Println("Already downloading file!")
	} else {
		fmt.Printf("DOWNLOADING metafile of %s from %s\n", packet.File, packet.Destination)

		watingChan := make(chan util.DataReply)
		defer close(watingChan)

		gossiper.lDownloadingChunk.mutex.Lock()
		gossiper.lDownloadingChunk.currentDownloadingChunks[metahash] = watingChan
		gossiper.lDownloadingChunk.mutex.Unlock()

		packetToSend := &util.GossipPacket{DataRequest: &util.DataRequest{
			Origin:      gossiper.Name,
			Destination: *packet.Destination,
			HopLimit:    util.HopLimit,
			HashValue:   *packet.Request,
		}}
		go gossiper.handleDataRequestPacket(packetToSend)
		for {
			ticker := time.NewTicker(5 * time.Second)
			select {
			case DataReply := <-watingChan:
				//fmt.Println(DataReply)
				hashBytes := DataReply.HashValue
				metahashToTest := hex.EncodeToString(hashBytes[:])
				metaFile := DataReply.Data
				if checkIntegrity(metahashToTest, metaFile) {
					util.WriteFileToArchive(metahash, metaFile)
					nbChunks := len(metaFile) / 32
					currentChunk := uint32(1)
					chunks := make([][]byte, 0, nbChunks)
					for i := 0; i < len(metaFile); i = i + 32 {
						chunks = append(chunks, metaFile[i:i+32])
					}
					fileCurrentStat := fileCurrentDownloadingStatus{
						currentChunkNumber: currentChunk,
						chunkHashes: chunks,
					}
					gossiper.lDownloads.mutex.Lock()
					gossiper.lDownloads.currentDownloads[metahash] = &fileCurrentStat
					gossiper.lDownloads.mutex.Unlock()

					gossiper.removeDownloadingChanel(metahash, watingChan)

					gossiper.downloadChunks(metahash, *packet.File, *packet.Destination)
					return
				} else {
					go gossiper.handleDataRequestPacket(packetToSend)
				}
			case <-ticker.C:
				go gossiper.handleDataRequestPacket(packetToSend)
			}
			ticker.Stop()
		}
	}
}

func (gossiper *Gossiper) getCurrStat(metahash string) (*fileCurrentDownloadingStatus,
	uint32, []byte, string) {

	gossiper.lDownloads.mutex.RLock()
	fileCurrentStat := gossiper.lDownloads.currentDownloads[metahash]
	gossiper.lDownloads.mutex.RUnlock()
	currNumber := fileCurrentStat.currentChunkNumber
	currHashBytes := fileCurrentStat.chunkHashes[fileCurrentStat.currentChunkNumber-1]
	currHash := hex.EncodeToString(currHashBytes)

	return fileCurrentStat, currNumber, currHashBytes, currHash
}

func (gossiper *Gossiper) incrementChunkNumber(fileCurrentStat *fileCurrentDownloadingStatus) uint32 {
	gossiper.lDownloads.mutex.Lock()
	currChunk := fileCurrentStat.currentChunkNumber + 1
	fileCurrentStat.currentChunkNumber = currChunk
	gossiper.lDownloads.mutex.Unlock()
	return currChunk
}

func (gossiper *Gossiper) downloadChunks(metahash string, fileName string, from string) {
	// TODO
	for {
		fileCurrentStat, _, currHashBytes, currHash := gossiper.getCurrStat(metahash)

		packetToSend := &util.GossipPacket{DataRequest: &util.DataRequest{
			Origin:      gossiper.Name,
			Destination: from,
			HopLimit:    util.HopLimit,
			HashValue:   currHashBytes,
		}}

		watingChan := make(chan util.DataReply)
		gossiper.lDownloadingChunk.mutex.Lock()
		gossiper.lDownloadingChunk.currentDownloadingChunks[currHash] = watingChan
		gossiper.lDownloadingChunk.mutex.Unlock()

		ticker := time.NewTicker(5 * time.Second)
		select {
		case DataReply := <-watingChan:
			hashBytes := DataReply.HashValue
			metahashToTest := hex.EncodeToString(hashBytes[:])
			metaFile := DataReply.Data
			if checkIntegrity(metahashToTest, metaFile) {
				currChunk := gossiper.incrementChunkNumber(fileCurrentStat)
				if int(currChunk) == len(fileCurrentStat.chunkHashes) {
					fmt.Printf("RECONSTRUCTED file %s\n", fileName)
					// TODO reconstruct
					gossiper.removeDownloadingChanel(currHash, watingChan)
					return
				}else {
					_, _, currHashBytes, _ := gossiper.getCurrStat(metahash)
					packetToSend.DataRequest.HashValue = currHashBytes
					go gossiper.handleDataRequestPacket(packetToSend)
				}
			} else {
				// TODO should we retry if data is corrupted ?
				go gossiper.handleDataRequestPacket(packetToSend)
			}

		case <-ticker.C:
			go gossiper.handleDataRequestPacket(packetToSend)
		}
		ticker.Stop()
		close(watingChan)
	}
}
