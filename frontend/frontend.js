
    let nBMessages = 0
    
    function getPeers(){
        $.ajax({
            type: "GET",
            url: "http://localhost:8080/node",
            dataType: 'json',
            success: function(data,status,xhr){
            if (data != {}) {
                data.sort();
                if (data.length > 0) {
                    document.getElementById("knownPeers").style.visibility = "visible";
                    var list = document.getElementById("peers")
                    while (list.hasChildNodes()) {
                        list.removeChild(list.lastChild);
                    }
                    for(let x of data){
                        $("#peers").append(
                            "<li>" +
                            x +
                            "</li>"
                        );
                    }
                }
            }
            }
        });
    }


  function getMessages(){
    $.ajax({
        type: "GET",
        url: "http://localhost:8080/message",
        dataType: 'json',
        success: function(data,status,xhr){
            if (data != {}) {
                if (data.length > 0) {
                    document.getElementById("chat").style.visibility = "visible";
                    var array = document.getElementById("messages")
                    for(let i = nBMessages; i < data.length; i++){
                        nBMessages = nBMessages + 1
                        $("#messages").find('tbody').append(
                            "<tr>" + "<th> FROM " + data[i].Origin + "</th>" +
                            "<th> Message " + data[i].Text + "</th>" +
                            "</tr>"
                        );
                      }
                  }
            }      
        }
      });
  }

  $.ajax({
    type: "GET",
    url: "http://localhost:8080/id",
    success: function(data,status,xhr){
      var name = JSON.parse(data);
      document.getElementById("nodeName").innerHTML = name.toString()
    }
  });

  setInterval(getMessages, 700);
  setInterval(getPeers, 700);

  getPeers()
  getMessages()

  function addNode(){
    var newNode = document.getElementById("sendNode").value;
    $.ajax({
        type: "POST",
        url: "http://localhost:8080/node",
        data: {"value":newNode},
        success: function(data,status,xhr){
            document.getElementById("sendNode").value = '';
        }
      })
  }

  function sendMessage(){
    var newMessage = document.getElementById("sendMessage").value;
    $.ajax({
        type: "POST",
        url: "http://localhost:8080/message",
        data: {"value":newMessage},
        success: function(data,status,xhr){
            document.getElementById("sendMessage").value = '';
        }
      })
  }

