package gossip

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/dpetresc/Peerster/util"
	"io"
	"math"
	"os"
	"strings"
	"sync"
	"time"
)

type LockFiles struct {
	// indexed and successfully downloaded files
	// fileName => MyFile
	Files map[string]*MyFile
	sync.RWMutex
}

type MyFile struct {
	fileName string
	fileSize int64
	Metafile [][]byte
	metahash string
	nbChunks uint64
}

type lockAllChunks struct {
	chunks map[string][]byte
	sync.RWMutex
}

type lockCurrentPublish struct {
	// id of TLC message => peers from wich it has received the acks
	acksPeers         map[uint32][]string
	majorityTrigger map[uint32]chan bool
	sync.RWMutex
}

func getInfoFile(f *os.File) (int64, int) {
	fileInfo, err := f.Stat()
	util.CheckError(err)
	fileSizeBytes := fileInfo.Size()
	nbChunks := int(math.Ceil(float64(fileSizeBytes) / float64(util.MaxUDPSize)))
	return fileSizeBytes, nbChunks
}

func (gossiper *Gossiper) createHashes(f *os.File) (int64, [][]byte, []byte) {
	fileSizeBytes, nbChunks := getInfoFile(f)

	chunkHashes := make([][]byte, 0, nbChunks)
	hashes := make([]byte, 0, 32*nbChunks)
	for i := int64(0); i < fileSizeBytes; i += int64(util.MaxUDPSize) {
		chunk := make([]byte, util.MaxUDPSize)
		if n, err := f.ReadAt(chunk, int64(i)); err != nil && err != io.EOF {
			util.CheckError(err)
		} else {
			shaBytes := sha256.Sum256(chunk[:n])
			sha := hex.EncodeToString(shaBytes[:])
			gossiper.lAllChunks.Lock()
			gossiper.lAllChunks.chunks[sha] = chunk[:n]
			gossiper.lAllChunks.Unlock()
			chunkHashes = append(chunkHashes, shaBytes[:])
			hashes = append(hashes, shaBytes[:]...)
		}
	}
	return fileSizeBytes, chunkHashes, hashes
}

func (gossiper *Gossiper) IndexFile(fileName string) *MyFile {
	f, err := os.Open(util.SharedFilesFolderPath + fileName)
	util.CheckError(err)
	defer f.Close()
	fileSizeBytes, chunkHashes, hashes := gossiper.createHashes(f)

	fileIdBytes := sha256.Sum256(hashes)
	metahash := hex.EncodeToString(fileIdBytes[:])

	gossiper.lAllChunks.Lock()
	gossiper.lAllChunks.chunks[metahash] = hashes
	gossiper.lAllChunks.Unlock()

	myFile := &MyFile{
		fileName: fileName,
		fileSize: fileSizeBytes,
		Metafile: chunkHashes,
		metahash: metahash,
		nbChunks: uint64(len(chunkHashes)),
	}
	fmt.Println("Metahash : " + metahash)
	gossiper.lFiles.Lock()
	gossiper.lFiles.Files[fileName] = myFile
	gossiper.lFiles.Unlock()

	if gossiper.hw3ex2 {
		txPublish := util.TxPublish{
			Name:         fileName,
			Size:         fileSizeBytes,
			MetafileHash: fileIdBytes[:],
		}
		blockPublish := util.BlockPublish{
			PrevHash:    [32]byte{},
			Transaction: txPublish,
		}
		packetToSend := gossiper.createNewTLCMessageToSend(blockPublish)
		fmt.Printf("UNCONFIRMED GOSSIP origin %s ID %d file name %s size %d metahash %s\n",
			gossiper.Name, packetToSend.TLCMessage.ID, fileName, fileSizeBytes, metahash)
		go gossiper.rumormonger("", "", &packetToSend, false)
		go gossiper.stubbornTLCMessage(&packetToSend)
	}

	return myFile
}

func (gossiper *Gossiper) stubbornTLCMessage(packet *util.GossipPacket) {
	ticker := time.NewTicker(time.Duration(gossiper.stubbornTimeout) * time.Second)
	gossiper.lCurrentPublish.RLock()
	majorityTriggerChan := gossiper.lCurrentPublish.majorityTrigger[packet.TLCMessage.ID]
	gossiper.lCurrentPublish.RUnlock()
	for {
		select {
		case <-ticker.C:
			go gossiper.rumormonger("", "", packet, false)
		case <-majorityTriggerChan:
			ticker.Stop()
			close(majorityTriggerChan)

			gossiper.lAllMsg.Lock()
			id := gossiper.lAllMsg.allMsg[gossiper.Name].GetNextID()
			packetToSend := util.GossipPacket{
				TLCMessage: &util.TLCMessage{
					Origin:      gossiper.Name,
					ID:          id,
					Confirmed:   int(packet.TLCMessage.ID),
					TxBlock:     packet.TLCMessage.TxBlock,
					VectorClock: packet.TLCMessage.VectorClock,
					Fitness:     packet.TLCMessage.Fitness,
				},
			}
			gossiper.lAllMsg.allMsg[gossiper.Name].AddMessage(&packetToSend, id, false)
			gossiper.lAllMsg.Unlock()

			gossiper.lCurrentPublish.Lock()
			if peers, ok := gossiper.lCurrentPublish.acksPeers[packet.TLCMessage.ID]; ok {
				fmt.Printf("RE-BROADCAST ID %d WITNESSES %s\n", packet.TLCMessage.ID, strings.Join(peers, ","))

				go gossiper.rumormonger("", "", &packetToSend, false)

				delete(gossiper.lCurrentPublish.majorityTrigger, packet.TLCMessage.ID)
				delete(gossiper.lCurrentPublish.acksPeers, packet.TLCMessage.ID)
				gossiper.lCurrentPublish.Unlock()
				return
			}
			// concurrency
			gossiper.lCurrentPublish.Unlock()
		}
	}
}