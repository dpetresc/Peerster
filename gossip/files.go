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
	// On peut avoir une seule fois à la fois
	from string
	hash string
}

type lockDownloadingChunks struct {
	currentDownloadingChunks map[DownloadIdentifier]chan util.DataReply
	sync.RWMutex
}

type lockCurrentDownloading struct {
	// DownloadIdentifier => chunk number
	currentDownloads map[DownloadIdentifier]uint64
	sync.RWMutex
}

type LockUncompletedFiles struct {
	// currently downloading files or finished downloading file but incomplete
	// filename => download identifier
	IncompleteFiles map[string]map[DownloadIdentifier]*MyFile
	sync.RWMutex
}


func (gossiper *Gossiper) alreadyHaveFileName(fileName string) bool {
	gossiper.lFiles.RLock()
	_, ok := gossiper.lFiles.Files[fileName]
	gossiper.lFiles.RUnlock()
	return ok
}

func (gossiper *Gossiper) isChunkAlreadyDownloaded(hash string) bool {
	gossiper.lAllChunks.RLock()
	_, ok := gossiper.lAllChunks.chunks[hash]
	gossiper.lAllChunks.RUnlock()
	return ok
}

func (gossiper *Gossiper) initWaitingChannel(chunkIdentifier DownloadIdentifier, hashBytes []byte, chunkNumber uint64,
	packet *util.Message, isMetaFile bool) chan util.DataReply {
	var waitingChan chan util.DataReply
	gossiper.lDownloadingChunk.Lock()
	if _, ok := gossiper.lDownloadingChunk.currentDownloadingChunks[chunkIdentifier]; !ok {
		// first time requesting this chunk
		waitingChan = make(chan util.DataReply, 100)
		gossiper.lDownloadingChunk.currentDownloadingChunks[chunkIdentifier] = waitingChan
	} else {
		waitingChan = gossiper.lDownloadingChunk.currentDownloadingChunks[chunkIdentifier]
	}
	gossiper.lDownloadingChunk.Unlock()
	packetToSend := &util.GossipPacket{DataRequest: &util.DataRequest{
		Origin:      gossiper.Name,
		Destination: chunkIdentifier.from,
		HopLimit:    util.HopLimit,
		HashValue:   hashBytes,
	}}
	if isMetaFile {
		fmt.Printf("DOWNLOADING metafile of %s from %s\n", *packet.File, chunkIdentifier.from)
	} else {
		fmt.Printf("DOWNLOADING %s chunk %d from %s\n", *packet.File, chunkNumber, chunkIdentifier.from)
	}
	go gossiper.handleDataRequestPacket(packetToSend)
	return waitingChan
}

func (gossiper *Gossiper) removeDownloadingChanel(chunkIdentifier DownloadIdentifier, channel chan util.DataReply) {
	gossiper.lDownloadingChunk.Lock()
	close(channel)
	delete(gossiper.lDownloadingChunk.currentDownloadingChunks, chunkIdentifier)
	gossiper.lDownloadingChunk.Unlock()
}

func getChunkHashes(metaFile []byte) [][]byte {
	nbChunks := len(metaFile) / 32
	chunks := make([][]byte, 0, nbChunks)
	for i := 0; i < len(metaFile); i = i + 32 {
		chunks = append(chunks, metaFile[i:i+32])
	}
	return chunks
}

func getHashAtChunkNumber(metaFile []byte, chunkNb uint64) []byte {
	// chunkNb starting from 1
	startIndex := (chunkNb - 1) * 32
	hash := metaFile[startIndex : startIndex+32]
	return hash
}

func (gossiper *Gossiper) incrementWaitingChunkNumber(downloadFileIdentifier DownloadIdentifier, currChunkNumber uint64) {
	// increment current chunk number
	gossiper.lCurrentDownloads.Lock()
	gossiper.lCurrentDownloads.currentDownloads[downloadFileIdentifier] = currChunkNumber + 1
	gossiper.lCurrentDownloads.Unlock()
}

func (gossiper *Gossiper) initCurrVars(downloadingMetaFile bool, packet *util.Message, metahash string,
	downloadFileIdentifier DownloadIdentifier, currChunkNumber uint64, from string) (string, []byte, DownloadIdentifier, int) {
	var currHash string
	var currHashByte []byte
	var currChunkIdentifier DownloadIdentifier
	var totalNbChunks int
	if downloadingMetaFile {
		currHashByte = *packet.Request
		currHash = metahash
		currChunkIdentifier = DownloadIdentifier{
			from: from,
			hash: downloadFileIdentifier.hash,
		}
	} else {
		gossiper.lAllChunks.RLock()
		hashes := gossiper.lAllChunks.chunks[metahash]
		gossiper.lAllChunks.RUnlock()
		totalNbChunks = len(hashes) / 32
		currHashByte = getHashAtChunkNumber(hashes, currChunkNumber)
		currHash = hex.EncodeToString(currHashByte)
		currChunkIdentifier = DownloadIdentifier{
			from: from,
			hash: currHash,
		}
	}
	return currHash, currHashByte, currChunkIdentifier, totalNbChunks
}

func (gossiper *Gossiper) startDownload(packet *util.Message) {
	metahash := hex.EncodeToString((*packet.Request)[:])

	if gossiper.alreadyHaveFileName(*packet.File) {
		// File name already exists, we do not overwrite it
		// Because we can not have more than one file with the same name
		fmt.Println("File name already exists, no overwrite of the file is performed")
		return
	}

	from := *packet.Destination
	downloadFileIdentifier := DownloadIdentifier{
		from: from,
		hash: metahash,
	}

	gossiper.lCurrentDownloads.Lock()
	if _, ok := gossiper.lCurrentDownloads.currentDownloads[downloadFileIdentifier]; ok {
		//fmt.Printf("Already downloading metahash %x from %s\n", *packet.Request, from)
		gossiper.lCurrentDownloads.Unlock()
	} else {
		gossiper.lCurrentDownloads.currentDownloads[downloadFileIdentifier] = 0
		gossiper.lCurrentDownloads.Unlock()

		for {
			gossiper.lCurrentDownloads.RLock()
			currChunkNumber := gossiper.lCurrentDownloads.currentDownloads[downloadFileIdentifier]
			gossiper.lCurrentDownloads.RUnlock()
			downloadingMetaFile := currChunkNumber == 0

			var waitingChan chan util.DataReply

			if downloadFileIdentifier.from == "" {
				gossiper.lSearchMatches.RLock()
				fileSearchIdentifier := FileSearchIdentifier{
					Filename: *packet.File,
					Metahash: metahash,
				}
				if matchStatus, ok := gossiper.lSearchMatches.Matches[fileSearchIdentifier]; !ok {
					// can't happen with the gui
					fmt.Println("Haven't search file/metahash before. Please try again after doing it")
					gossiper.lSearchMatches.RUnlock()
					gossiper.finishDownload(packet, metahash, downloadFileIdentifier, false)
					return
				}else {
					chunkWanted := currChunkNumber
					if downloadingMetaFile {
						// if the metafile is wanted request to a peer that has the first chunk
						chunkWanted = uint64(1)
					}
					peersHavingChunk, ok := matchStatus.chunksDistribution[chunkWanted];
					if !ok || len(peersHavingChunk) == 0 {
						// the search request haven't finished (no SEARCH FINISHED printed)
						// => incomplete download
						gossiper.lSearchMatches.RUnlock()
						gossiper.finishDownload(packet, metahash, downloadFileIdentifier, false)
						return
					} else {
						from = peersHavingChunk[0]
					}
				}
				gossiper.lSearchMatches.RUnlock()
			}
			currHash, currHashByte, currChunkIdentifier, totalNbChunks := gossiper.initCurrVars(downloadingMetaFile,
				packet, metahash, downloadFileIdentifier, currChunkNumber, from)
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
						gossiper.lAllChunks.Lock()
						gossiper.lAllChunks.chunks[hashToTest] = data
						gossiper.lAllChunks.Unlock()

						if gossiper.updateFileStructures(downloadingMetaFile, data, packet,
							metahash, downloadFileIdentifier, currChunkNumber, totalNbChunks) {
							return
						}
					} else {
						// if the data is empty we skip and finish download
						// should not happen for the case when we download after a search request
						gossiper.finishDownload(packet, metahash, downloadFileIdentifier, false)
						return
					}
				case <-ticker.C:
					ticker.Stop()
				}
			} else {
				// we have already downloaded the chunk so we need to download the next chunk
				gossiper.lAllChunks.RLock()
				data, _ := gossiper.lAllChunks.chunks[currHash]
				gossiper.lAllChunks.RUnlock()
				if gossiper.updateFileStructures(downloadingMetaFile, data, packet,
					metahash, downloadFileIdentifier, currChunkNumber, totalNbChunks) {
					return
				}
			}
		}
	}
}

func (gossiper *Gossiper) updateFileStructures(downloadingMetaFile bool, data []byte, packet *util.Message,
	metahash string, downloadFileIdentifier DownloadIdentifier, currChunkNumber uint64, totalNbChunks int) bool {
	if downloadingMetaFile {
		chunkHashes := getChunkHashes(data)
		fileStruct := MyFile{
			fileName: *packet.File,
			fileSize: -1,
			Metafile: chunkHashes,
			metahash: metahash,
			nbChunks: 0,
		}
		// We can not have several same downloadFileIdentifier at the same time
		// If we already have one we need to override it because it was an old (incomplete) download
		gossiper.lUncompletedFiles.Lock()
		if _, ok := gossiper.lUncompletedFiles.IncompleteFiles[*packet.File]; !ok {
			gossiper.lUncompletedFiles.IncompleteFiles[*packet.File] = make(map[DownloadIdentifier]*MyFile)
		}
		gossiper.lUncompletedFiles.IncompleteFiles[*packet.File][downloadFileIdentifier] = &fileStruct
		gossiper.lUncompletedFiles.Unlock()
	} else {
		gossiper.lUncompletedFiles.Lock()
		gossiper.lUncompletedFiles.IncompleteFiles[*packet.File][downloadFileIdentifier].nbChunks = currChunkNumber
		gossiper.lUncompletedFiles.Unlock()

		if int(currChunkNumber) >= totalNbChunks {
			gossiper.finishDownload(packet, metahash, downloadFileIdentifier, true)
			return true
		}
	}
	gossiper.incrementWaitingChunkNumber(downloadFileIdentifier, currChunkNumber)
	return false
}

func (gossiper *Gossiper) finishDownload(packet *util.Message, metahash string,
	downloadFileIdentifier DownloadIdentifier, success bool) {
	if success {
		fmt.Printf("RECONSTRUCTED file %s\n", *packet.File)
		gossiper.reconstructFile(metahash, *packet.File, downloadFileIdentifier)
	}

	// remove from current downloading list
	gossiper.lCurrentDownloads.Lock()
	delete(gossiper.lCurrentDownloads.currentDownloads, downloadFileIdentifier)
	gossiper.lCurrentDownloads.Unlock()
}

func (gossiper *Gossiper) reconstructFile(metahash string, fileName string, downloadFileIdentifier DownloadIdentifier) {
	filePath := util.DownloadsFolderPath + fileName
	if _, err := os.Stat(filePath); err == nil {
		// File name already exists, we do not overwrite it
		// Should not happen as we already check it above
		fmt.Println("File name already exists, no overwrite of the file is performed")
		return
	}

	gossiper.lAllChunks.RLock()
	hashes, _ := gossiper.lAllChunks.chunks[metahash]
	gossiper.lAllChunks.RUnlock()
	chunkHashes := getChunkHashes(hashes)

	fileBytes := make([]byte, 0)
	for _, hashBytes := range chunkHashes {
		hash := hex.EncodeToString(hashBytes)

		gossiper.lAllChunks.RLock()
		if data, ok := gossiper.lAllChunks.chunks[hash]; !ok {
			// should never happen
			fmt.Println("PROBLEM WITH DOWNLOAD !")
			os.Exit(1)
		} else {
			gossiper.lAllChunks.RUnlock()
			fileBytes = append(fileBytes, data...)
		}
	}
	file, err := os.Create(filePath)
	util.CheckError(err)
	_, err = file.Write(fileBytes)
	util.CheckError(err)
	if err == nil {
		fileStruct := MyFile{
			fileName: fileName,
			fileSize: int64(len(fileBytes)),
			Metafile: chunkHashes,
			metahash: metahash,
			nbChunks: uint64(len(chunkHashes)),
		}
		gossiper.lFiles.Lock()
		gossiper.lFiles.Files[fileName] = &fileStruct
		gossiper.lFiles.Unlock()

		gossiper.lUncompletedFiles.Lock()
		delete(gossiper.lUncompletedFiles.IncompleteFiles[fileName], downloadFileIdentifier)
		gossiper.lUncompletedFiles.Unlock()
	}
	err = file.Close()
	util.CheckError(err)
}