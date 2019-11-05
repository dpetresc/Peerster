package gossip

import (
	"encoding/hex"
	"fmt"
	"github.com/dpetresc/Peerster/util"
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

type lockCurrentDownloading struct {
	// DownloadIdentifier => chunk number
	currentDownloads map[DownloadIdentifier]uint32
	mutex sync.RWMutex
}

func (gossiper *Gossiper) alreadyHaveFile(metahash string) bool {
	gossiper.lFiles.Mutex.RLock()
	_, ok := gossiper.lFiles.Files[metahash]
	gossiper.lFiles.Mutex.RUnlock()
	return ok
}

func (gossiper *Gossiper) isChunkAlreadyDownloaded(hash string) bool {
	gossiper.lAllChunks.mutex.RLock()
	_, ok := gossiper.lAllChunks.chunks[hash]
	gossiper.lAllChunks.mutex.RUnlock()
	return ok
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

func getHashAtChunkNumber(metaFile []byte, chunkNb uint32) []byte {
	// chunkNb starting from 1
	startIndex := (chunkNb-1)*32
	hash := metaFile[startIndex:startIndex+32]
	return hash
}

func (gossiper *Gossiper) startDownload(packet *util.Message){
	metahash := hex.EncodeToString((*packet.Request)[:])

	if gossiper.alreadyHaveFile(metahash) {
		fmt.Printf("RECONSTRUCTED file %s\n", *packet.File)
		gossiper.reconstructFile(metahash, *packet.File)
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
		//fmt.Printf("Already downloading metahash %x from %s\n", *packet.Request, from)
		// TODO pending requests => keep track of those requests or just ignore them
		gossiper.lCurrentDownloads.mutex.Unlock()
	} else {
		gossiper.lCurrentDownloads.currentDownloads[downloadFileIdentifier] = 0
		gossiper.lCurrentDownloads.mutex.Unlock()

		for {
			gossiper.lCurrentDownloads.mutex.RLock()
			currChunkNumber := gossiper.lCurrentDownloads.currentDownloads[downloadFileIdentifier]
			gossiper.lCurrentDownloads.mutex.RUnlock()
			downloadingMetaFile := currChunkNumber == 0

			var waitingChan chan util.DataReply

			var currHash string
			var currHashByte []byte
			var currChunkIdentifier DownloadIdentifier
			var totalNbChunks int
			if downloadingMetaFile {
				currHashByte = *packet.Request
				currHash = metahash
				currChunkIdentifier = downloadFileIdentifier
			}else {
				gossiper.lAllChunks.mutex.RLock()
				hashes := gossiper.lAllChunks.chunks[metahash]
				gossiper.lAllChunks.mutex.RUnlock()
				totalNbChunks = len(hashes) / 32
				currHashByte = getHashAtChunkNumber(hashes, currChunkNumber)
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
					packet, downloadingMetaFile)
				ticker := time.NewTicker(5 * time.Second)
				select {
				case dataReply := <-waitingChan:
					gossiper.removeDownloadingChanel(currChunkIdentifier, waitingChan)
					ticker.Stop()

					hashBytes := dataReply.HashValue
					hashToTest := hex.EncodeToString(hashBytes[:])
					data := make([]byte, 0, len(dataReply.Data))
					data = append(data, dataReply.Data...)
					if len(data) != 0 {
						// successful download
						gossiper.lAllChunks.mutex.Lock()
						gossiper.lAllChunks.chunks[hashToTest] = data
						gossiper.lAllChunks.mutex.Unlock()

						// increment current chunk number
						gossiper.lCurrentDownloads.mutex.RLock()
						gossiper.lCurrentDownloads.currentDownloads[downloadFileIdentifier] = currChunkNumber + 1
						gossiper.lCurrentDownloads.mutex.RUnlock()

						if !downloadingMetaFile {
							if int(currChunkNumber) >= totalNbChunks {
								fmt.Printf("RECONSTRUCTED file %s\n", *packet.File)
								gossiper.reconstructFile(metahash, *packet.File)

								// remove from current downloading list
								gossiper.lCurrentDownloads.mutex.Lock()
								delete(gossiper.lCurrentDownloads.currentDownloads, downloadFileIdentifier)
								gossiper.lCurrentDownloads.mutex.Unlock()
								return
							}
						}
					} else {
						// if the data is empty we skip
						// finish download
						// remove from current downloading list
						gossiper.lCurrentDownloads.mutex.Lock()
						delete(gossiper.lCurrentDownloads.currentDownloads, downloadFileIdentifier)
						gossiper.lCurrentDownloads.mutex.Unlock()
						return
					}
				case <-ticker.C:
					ticker.Stop()
				}
			} else {
				// we have already downloaded the chunk so we need to download the next chunk

				// increment current chunk number
				gossiper.lCurrentDownloads.mutex.RLock()
				gossiper.lCurrentDownloads.currentDownloads[downloadFileIdentifier] = currChunkNumber + 1
				gossiper.lCurrentDownloads.mutex.RUnlock()

				if !downloadingMetaFile {
					if int(currChunkNumber) >= totalNbChunks {
						fmt.Printf("RECONSTRUCTED file %s\n", *packet.File)
						gossiper.reconstructFile(metahash, *packet.File)

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

func (gossiper *Gossiper) reconstructFile(metahash string, fileName string) {
	filePath := util.DownloadsFolderPath + fileName
	if _, err := os.Stat(filePath); err == nil {
		// File name already exists, we do not overwrite it
		// Could be the same metahash or another
		//fmt.Println("File name already exists, no overwrite of the file is performed")
		return
	}

	gossiper.lAllChunks.mutex.RLock()
	hashes, _ := gossiper.lAllChunks.chunks[metahash]
	gossiper.lAllChunks.mutex.RUnlock()
	chunkHashes := getChunkHashes(hashes)

	fileBytes := make([]byte, 0)
	for _, hashBytes := range chunkHashes {
		hash := hex.EncodeToString(hashBytes)

		gossiper.lAllChunks.mutex.RLock()
		data, ok := gossiper.lAllChunks.chunks[hash]
		if !ok {
			// should never happen
			//fmt.Println("PROBLEM WITH DOWNLOAD !")
			//os.Exit(1)
		}
		gossiper.lAllChunks.mutex.RUnlock()
		fileBytes = append(fileBytes, data...)
	}
	file, err := os.Create(filePath)
	util.CheckError(err)
	_, err = file.Write(fileBytes)
	util.CheckError(err)
	if err == nil {
		if !gossiper.alreadyHaveFile(metahash) {
			fileStruct := MyFile{
				fileName: fileName,
				fileSize: int64(len(fileBytes)),
				Metafile: chunkHashes,
				metahash: metahash,
			}
			gossiper.lFiles.Mutex.Lock()
			gossiper.lFiles.Files[metahash] = &fileStruct
			gossiper.lFiles.Mutex.Unlock()
		}
	}
	err = file.Close()
	util.CheckError(err)
}