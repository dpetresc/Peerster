package gossip

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/dpetresc/Peerster/util"
	"io/ioutil"
	"os"
	"sync"
	"time"
)

type DownloadIdentifier struct {
	from string
	hash string
}

type lockDownloadingChunks struct {
	currentDownloadingChunks map[DownloadIdentifier]chan util.DataReply
	mutex sync.Mutex
}

type fileCurrentDownloadingStatus struct {
	currentChunkNumber uint32
	chunkHashes [][]byte
}

type lockCurrentDownloading struct {
	currentDownloads map[DownloadIdentifier]*fileCurrentDownloadingStatus
	mutex sync.RWMutex
}

type lockDownloadedFiles struct {
	// keep track of all downloaded files so far (metahash) (downloads finished)
	downloads map[string]bool
	mutex     sync.RWMutex
}

func checkIntegrity(hash string, data []byte) bool {
	sha := sha256.Sum256(data[:])
	hashToTest := hex.EncodeToString(sha[:])
	return hashToTest == hash
}

func (gossiper *Gossiper) isFileAlreadyDownloaded(metahash string) bool {
	gossiper.lDownloadedFiles.mutex.Lock()
	_, ok := gossiper.lDownloadedFiles.downloads[metahash]
	gossiper.lDownloadedFiles.mutex.Unlock()
	return ok
}

func (gossiper *Gossiper) isChunkAlreadyDownloaded(hash string) bool {
	chunkPath := util.ChunksFolderPath + hash + ".bin"
	if _, err := os.Stat(chunkPath); err == nil {
		return true
	}
	return false
}

func (gossiper *Gossiper) initWaitingChannel(chunkIdentifier DownloadIdentifier, hashBytes []byte, chunkNumber uint32,
	packet *util.Message, isMetaFile bool) (chan util.DataReply) {
	var waitingChan chan util.DataReply
	gossiper.lDownloadingChunk.mutex.Lock()
	_, ok := gossiper.lDownloadingChunk.currentDownloadingChunks[chunkIdentifier]
	if !ok {
		// first time requesting this chunk
		waitingChan = make(chan util.DataReply)
		gossiper.lDownloadingChunk.currentDownloadingChunks[chunkIdentifier] = waitingChan
	} else {
		waitingChan = gossiper.lDownloadingChunk.currentDownloadingChunks[chunkIdentifier]
	}
	gossiper.lDownloadingChunk.mutex.Unlock()
	packetToSend := &util.GossipPacket{DataRequest: &util.DataRequest{
		Origin:      gossiper.Name,
		Destination: *packet.Destination,
		HopLimit:    util.HopLimit,
		HashValue:   hashBytes,
	}}
	if isMetaFile {
		fmt.Printf("DOWNLOADING metafile of %s from %s\n", *packet.File, *packet.Destination)
	} else {
		fmt.Printf("DOWNLOADING %s chunk %d from %s\n", *packet.File, chunkNumber, *packet.Destination)
	}
	go gossiper.handleDataRequestPacket(packetToSend)
	return waitingChan
}

func (gossiper *Gossiper) removeDownloadingChanel(chunkIdentifier DownloadIdentifier, channel chan util.DataReply) {
	gossiper.lDownloadingChunk.mutex.Lock()
	close(channel)
	delete(gossiper.lDownloadingChunk.currentDownloadingChunks, chunkIdentifier)
	gossiper.lDownloadingChunk.mutex.Unlock()
}

func getChunkHashes(metaFile []byte) [][]byte {
	nbChunks := len(metaFile) / 32
	chunks := make([][]byte, 0, nbChunks)
	for i := 0; i < len(metaFile); i = i + 32 {
		chunks = append(chunks, metaFile[i:i+32])
	}
	return chunks
}

func (gossiper *Gossiper) initFileCurrentStat(downloadFileIdentifier DownloadIdentifier, metaFile []byte) {
	chunks := getChunkHashes(metaFile)
	currentChunk := uint32(1)
	fileCurrentStat := fileCurrentDownloadingStatus{
		currentChunkNumber: currentChunk,
		chunkHashes:        chunks,
	}
	gossiper.lCurrentDownloads.mutex.Lock()
	gossiper.lCurrentDownloads.currentDownloads[downloadFileIdentifier] = &fileCurrentStat
	gossiper.lCurrentDownloads.mutex.Unlock()
}

func (gossiper *Gossiper) incrementChunkNumber(fileCurrentStat *fileCurrentDownloadingStatus) uint32 {
	gossiper.lCurrentDownloads.mutex.Lock()
	currChunk := fileCurrentStat.currentChunkNumber + 1
	fileCurrentStat.currentChunkNumber = currChunk
	gossiper.lCurrentDownloads.mutex.Unlock()
	return currChunk
}

func (gossiper *Gossiper) startDownload(packet *util.Message){
	metahash := hex.EncodeToString((*packet.Request)[:])

	if gossiper.isFileAlreadyDownloaded(metahash) {
		metahashPath := util.ChunksFolderPath + metahash + ".bin"
		data, err := ioutil.ReadFile(metahashPath)
		util.CheckError(err)
		fmt.Printf("RECONSTRUCTED file %s\n", *packet.File)
		gossiper.reconstructFile(metahash, *packet.File, getChunkHashes(data))
		return
	}

	from := *packet.Destination

	downloadFileIdentifier := DownloadIdentifier{
		from: from,
		hash: metahash,
	}

	gossiper.lCurrentDownloads.mutex.Lock()
	_, ok := gossiper.lCurrentDownloads.currentDownloads[downloadFileIdentifier]
	if ok {
		fmt.Printf("Already downloading metahash %x from %s\n", *packet.Request, from)
		// TODO pending requests => keep track of those requests or just ignore them ?
		gossiper.lCurrentDownloads.mutex.Unlock()
	} else {
		gossiper.lCurrentDownloads.currentDownloads[downloadFileIdentifier] = nil
		gossiper.lCurrentDownloads.mutex.Unlock()

		// number of times we tried to download a chunk and it was corrupted
		failedAttempt := 0

		for {
			// TODO lock ?
			gossiper.lCurrentDownloads.mutex.RLock()
			fileCurrentStat := gossiper.lCurrentDownloads.currentDownloads[downloadFileIdentifier]
			isMetaFile := fileCurrentStat == nil
			gossiper.lCurrentDownloads.mutex.RUnlock()

			var waitingChan chan util.DataReply

			var currHash string
			var currHashByte []byte
			var currChunkNumber uint32
			var currChunkIdentifier DownloadIdentifier
			if isMetaFile {
				currHashByte = *packet.Request
				currHash = metahash
				currChunkNumber = 0
				currChunkIdentifier = downloadFileIdentifier
			}else {
				currChunkNumber = fileCurrentStat.currentChunkNumber
				currHashByte = fileCurrentStat.chunkHashes[currChunkNumber-1]
				currHash = hex.EncodeToString(currHashByte)
				currChunkIdentifier = DownloadIdentifier{
					from: from,
					hash: currHash,
				}
			}
			// check if we already have this chunk from another download
			if !gossiper.isChunkAlreadyDownloaded(currHash) {
				// we have to download the chunk
				waitingChan = gossiper.initWaitingChannel(currChunkIdentifier, currHashByte, currChunkNumber,
					packet, isMetaFile)
				ticker := time.NewTicker(5 * time.Second)
				select {
				case dataReply := <-waitingChan:
					hashBytes := dataReply.HashValue
					hashToTest := hex.EncodeToString(hashBytes[:])
					data := dataReply.Data
					if checkIntegrity(hashToTest, data) {
						// successful download
						failedAttempt = 0

						file := util.WriteFileToArchive(hashToTest, data)
						file.Close()

						if isMetaFile {
							gossiper.initFileCurrentStat(currChunkIdentifier, data)
						} else {
							currChunk := gossiper.incrementChunkNumber(fileCurrentStat)
							if int(currChunk) == len(fileCurrentStat.chunkHashes) {
								fmt.Printf("RECONSTRUCTED file %s\n", *packet.File)
								gossiper.reconstructFile(metahash, *packet.File, fileCurrentStat.chunkHashes)
								gossiper.removeDownloadingChanel(currChunkIdentifier, waitingChan)

								// remove from current downloading list
								gossiper.lCurrentDownloads.mutex.Lock()
								delete(gossiper.lCurrentDownloads.currentDownloads, downloadFileIdentifier)
								gossiper.lCurrentDownloads.mutex.Unlock()
								return
							}
						}
						gossiper.removeDownloadingChanel(currChunkIdentifier, waitingChan)
					} else {
						// corrupted or empty
						// if the data is empty we skip
						// if it is corrupted we try again (max 5 times)
						failedAttempt = failedAttempt + 1
						if len(dataReply.Data) == 0 || failedAttempt >= 4{
							// finish download
							gossiper.removeDownloadingChanel(currChunkIdentifier, waitingChan)

							// remove from current downloading list
							gossiper.lCurrentDownloads.mutex.Lock()
							delete(gossiper.lCurrentDownloads.currentDownloads, downloadFileIdentifier)
							gossiper.lCurrentDownloads.mutex.Unlock()
							return
						}
					}
				case <-ticker.C:
					ticker.Stop()
				}
			} else {
				// we have already downloaded the chunk so we need to download the next chunk
				if isMetaFile {
					metahashPath := util.ChunksFolderPath + metahash + ".bin"
					data, err := ioutil.ReadFile(metahashPath)
					util.CheckError(err)
					gossiper.initFileCurrentStat(currChunkIdentifier, data)
				} else {
					currChunk := gossiper.incrementChunkNumber(fileCurrentStat)
					if int(currChunk) == len(fileCurrentStat.chunkHashes) {
						fmt.Printf("RECONSTRUCTED file %s\n", *packet.File)
						gossiper.reconstructFile(metahash, *packet.File, fileCurrentStat.chunkHashes)

						// remove from current downloading list
						gossiper.lCurrentDownloads.mutex.Lock()
						delete(gossiper.lCurrentDownloads.currentDownloads, downloadFileIdentifier)
						gossiper.lCurrentDownloads.mutex.Unlock()
						return
					}
				}
			}
		}
	}
}

func (gossiper *Gossiper) reconstructFile(metahash string, fileName string, chunkHashes [][]byte) {
	filePath := util.DownloadsFolderPath + fileName

	if _, err := os.Stat(filePath); err == nil {
		// File name already exists, we do not overwrite it
		// Could be the same metahash or another
		fmt.Println("File name already exists, no overwrite of the file is performed")
		return
	}

	fileBytes := make([]byte, 0)
	for _, hashBytes := range chunkHashes {
		hash := hex.EncodeToString(hashBytes)
		chunkPath := util.ChunksFolderPath + hash + ".bin"
		var data []byte
		var err error = nil
		if _, err = os.Stat(chunkPath); err == nil {
			data, err = ioutil.ReadFile(chunkPath)
			fileBytes = append(fileBytes, data...)
		}
		util.CheckError(err)
		if err != nil {
			return
		}
	}
	file, err := os.Create(filePath)
	util.CheckError(err)
	_, err = file.Write(fileBytes)
	util.CheckError(err)
	if err == nil {
		gossiper.lDownloadedFiles.mutex.Lock()
		gossiper.lDownloadedFiles.downloads[metahash] = true
		gossiper.lDownloadedFiles.mutex.Unlock()
	}
	err = file.Close()
	util.CheckError(err)
}