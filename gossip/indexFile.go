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

// GUI
type Confirmation struct {
	Origin string
	ID     uint32
}

type lockCurrentPublish struct {
	// id of TLC message => peers from wich it has received the acks
	acksPeers         map[uint32][]string
	majorityTrigger map[uint32]chan bool
	MyRound      uint32
	OtherRounds  map[string]uint32
	Queue        []*MyFile
	Confirmed    map[uint32][]Confirmation
	FutureMsg    []*util.TLCMessage
	LastID       uint32
	ReicvCommand bool

	GuiMessages []string
	sync.RWMutex
}

func (gossiper *Gossiper) Enqueue(queue []*MyFile, myfile *MyFile)[]*MyFile{
	return append(queue, myfile)
}

func (gossiper *Gossiper) Dequeue(queue []*MyFile) (*MyFile, []*MyFile){
	if len(queue) == 0{
		return nil, queue
	}else{
		result := queue[0]
		return result, queue[1:]
	}
}
// END GUI

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

	if gossiper.hw3ex2 || gossiper.hw3ex3 {
		if gossiper.hw3ex2 || (gossiper.ackAll && gossiper.hw3ex3) {
			gossiper.BroadcastNewFile(myFile)
		} else if !gossiper.LCurrentPublish.ReicvCommand {
			gossiper.LCurrentPublish.Lock()
			gossiper.LCurrentPublish.ReicvCommand = true
			gossiper.LCurrentPublish.Unlock()
			gossiper.BroadcastNewFile(myFile)
			gossiper.LCurrentPublish.Lock()
			gossiper.TryNextRound()
			gossiper.LCurrentPublish.Unlock()
		} else {
			gossiper.LCurrentPublish.Lock()
			gossiper.Enqueue(gossiper.LCurrentPublish.Queue, myFile)
			gossiper.LCurrentPublish.Unlock()
		}
	}

	return myFile
}

func (gossiper *Gossiper) BroadcastNewFile(myFile *MyFile) {
	fileIdBytes, err := hex.DecodeString(myFile.metahash)
	util.CheckError(err)
	txPublish := util.TxPublish{
		Name:         myFile.fileName,
		Size:         myFile.fileSize,
		MetafileHash: fileIdBytes,
	}
	blockPublish := util.BlockPublish{
		PrevHash:    [32]byte{},
		Transaction: txPublish,
	}
	packetToSend := gossiper.createNewTLCMessageToSend(blockPublish)
	fmt.Printf("UNCONFIRMED GOSSIP origin %s ID %d file name %s size %d metahash %s\n",
		gossiper.Name, packetToSend.TLCMessage.ID, myFile.fileName, myFile.fileSize, myFile.metahash)
	go gossiper.rumormonger("", "", &packetToSend, false)
	go gossiper.stubbornTLCMessage(&packetToSend)
}

func (gossiper *Gossiper) stubbornTLCMessage(packet *util.GossipPacket) {
	ticker := time.NewTicker(time.Duration(gossiper.stubbornTimeout) * time.Second)
	gossiper.LCurrentPublish.RLock()
	majorityTriggerChan := gossiper.LCurrentPublish.majorityTrigger[packet.TLCMessage.ID]
	gossiper.LCurrentPublish.RUnlock()
	for {
		select {
		case <-ticker.C:
			gossiper.rumormonger("", "", packet, false)
		case majority := <-majorityTriggerChan:
			ticker.Stop()
			close(majorityTriggerChan)
			fmt.Println(majority)
			if majority {
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

				gossiper.LCurrentPublish.Lock()
				if peers, ok := gossiper.LCurrentPublish.acksPeers[packet.TLCMessage.ID]; ok {
					fmt.Printf("RE-BROADCAST ID %d WITNESSES %s\n", packet.TLCMessage.ID, strings.Join(peers, ","))

					gossiper.rumormonger("", "", &packetToSend, false)

					delete(gossiper.LCurrentPublish.majorityTrigger, packet.TLCMessage.ID)
					delete(gossiper.LCurrentPublish.acksPeers, packet.TLCMessage.ID)
				}
				// concurrency
				if gossiper.hw3ex3 && !gossiper.ackAll{
					gossiper.LCurrentPublish.Confirmed[gossiper.LCurrentPublish.MyRound] = append(gossiper.LCurrentPublish.Confirmed[gossiper.LCurrentPublish.MyRound], Confirmation{
						Origin: gossiper.Name,
						ID:     packetToSend.TLCMessage.ID,
					})
					gossiper.TryNextRound()
				}
				gossiper.LCurrentPublish.Unlock()
			}
			return
		}
	}
}

func (g *Gossiper) TryNextRound() {
	tm := g.LCurrentPublish
	confirmed := tm.Confirmed[tm.MyRound]
	if len(confirmed) > int(g.N)/2 && tm.ReicvCommand {
		tm.MyRound += 1
		g.PrintNextRound(tm.MyRound, confirmed)

		if len(tm.acksPeers[tm.LastID]) <= int(g.N)/2 {
			tm.majorityTrigger[tm.LastID] <- false
		}

		metadata, queue := g.Dequeue(tm.Queue)
		tm.Queue = queue
		if metadata != nil {
			g.BroadcastNewFile(metadata)
		} else {
			tm.ReicvCommand = false
		}
	}
}

func (g *Gossiper) PrintNextRound(nextRound uint32, confirmations []Confirmation) {
	conf := ""
	//origin1 <origin> ID1 <ID>, origin2 <origin> ID2 <ID>
	for i, c := range confirmations {
		if i < len(confirmations)-1{
			conf += fmt.Sprintf(" origin%d %s ID%d %d,", i+1, c.Origin, i+1, c.ID)

		}else{
			conf += fmt.Sprintf(" origin%d %s ID%d %d", i+1, c.Origin, i+1, c.ID)

		}
	}

	s := fmt.Sprintf("ADVANCING TO %d round BASED ON CONFIRMED MESSAGES%s\n", nextRound, conf)

	g.LCurrentPublish.GuiMessages = append(g.LCurrentPublish.GuiMessages, s)

	fmt.Printf(s)
}