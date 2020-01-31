let currentChat = "public"
let cID = "nil"

/****************************** GET ******************************/
function getPeers() {
    $.ajax({
        type: "GET",
        url: "http://localhost:8080/node",
        dataType: 'json',
        success: function (data, status, xhr) {
            if (data != {}) {
                data.sort();
                if (data.length > 0) {
                    var list = document.getElementById("peers")
                    while (list.hasChildNodes()) {
                        list.removeChild(list.lastChild);
                    }
                    for (let x of data) {
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


function getMessages() {
    $.ajax({
        type: "GET",
        url: "http://localhost:8080/message",
        dataType: 'json',
        success: function (data, status, xhr) {
            if (data != {}) {
                if (data.length > 0) {
                    for (let i = 0; i < data.length; i++) {
                        if (verifyMessage(data[i].Text)) {
                            $("#public_messages").find("tbody").append(
                                "<tr>" + "<th> From " + "<span style=\"font-weight:normal\">" +
                                data[i].Origin + "<\span>" + "</th>" +
                                "<th> Message " + "<span style=\"font-weight:normal\">" +
                                data[i].Text + "<\span>" + "</th>" +
                                "</tr>"
                            );
                        }
                    }
                }
            }
        }
    });
}

function createTableIfNotExist(x) {
    let elemExists = document.getElementById("private_" + x)
    if (elemExists == null) {
        var div = document.createElement('div');
        div.style.visibility = "visible"
        div.id = "private_" + x
        var tbl = document.createElement('table')
        tbl.id = "private_" + x + "_messages"
        tbl.style.visibility = "collapse"
        tbl.appendChild(document.createElement('tbody'))
        div.appendChild(tbl)
        document.getElementById("chat_table").appendChild(div)
    }
    return elemExists == null
}

function getNodeIdentifiers() {
    $.ajax({
        type: "GET",
        url: "http://localhost:8080/identifier",
        dataType: 'json',
        success: function (data, status, xhr) {
            if (data != {}) {
                data.sort();
                if (data.length > 0) {
                    document.getElementById("knownIds").style.visibility = "visible";
                    var list = document.getElementById("ids")
                    var oldChildStyle = {}
                    while (list.hasChildNodes()) {
                        var oldChild = list.removeChild(list.lastChild);
                        oldChildStyle[oldChild.innerHTML] = oldChild.style
                    }
                    for (let x of data) {
                        $("#ids").append(
                            "<li id=\"identifier_" + x + "\"" + ">" +
                            x +
                            "</li>"
                        );
                        if (oldChildStyle[x] != undefined) {
                            document.getElementById("identifier_" + x).style.color = oldChildStyle[x].color
                            document.getElementById("identifier_" + x).style.fontWeight = oldChildStyle[x].fontWeight
                        } else {
                            createTableIfNotExist(x)
                        }
                        $("#identifier_" + x).click(function () {
                            if (currentChat === x.toString()) {
                                // go back to public chat
                                currentChat = "public"
                                document.getElementById("identifier_" + x).style.fontWeight = "normal"
                                document.getElementById("chat").innerText = "Public Chat"
                                document.getElementById("sendMessage").placeholder = "Type your public message here"

                                /*document.getElementById("fileRequest").placeholder = "Please first select a node for the request"
                                document.getElementById("hashRequest").placeholder = "Please first select a node for the request"*/

                                document.getElementById("private_" + x + "_messages").style.visibility = "collapse"
                                document.getElementById("public_messages").style.visibility = "visible"
                            } else if (currentChat === "public") {
                                // change red color that notified new messages if necessary
                                document.getElementById("identifier_" + x).style.color = "black"

                                currentChat = x.toString()
                                document.getElementById("identifier_" + x).style.fontWeight = "bold"
                                document.getElementById("chat").innerHTML = "Private Chat" + "<br/>" + x
                                document.getElementById("sendMessage").placeholder = "Type your private message for " + currentChat + " here"

                                /*document.getElementById("fileRequest").placeholder = "Type the name of the downloaded file"
                                document.getElementById("hashRequest").placeholder = "Type metahash of the file request for " + currentChat + " here"*/

                                document.getElementById("public_messages").style.visibility = "collapse"
                                document.getElementById("private_" + x + "_messages").style.visibility = "visible"
                            }
                        });

                    }
                }
            }
        }
    });
}

function getPrivateMessages() {
    // refresh list of identifiers
    getNodeIdentifiers()
    $.ajax({
        type: "GET",
        url: "http://localhost:8080/private",
        dataType: 'json',
        success: function (data, status, xhr) {
            if (data != undefined) {
                for (let x in data) {
                    if (x != currentChat) {
                        document.getElementById("identifier_" + x).style.color = "red"
                        document.getElementById("identifier_" + x).style.fontWeight = "bold"
                    }
                    for (let msg of data[x]) {
                        if (verifyMessage(msg.Text)) {
                            $("#private_" + x + "_messages").find("tbody").append(
                                "<tr>" + "<th> From " + "<span style=\"font-weight:normal\">" +
                                msg.Origin + "<\span>" + "</th>" +
                                "<th> Message " + "<span style=\"font-weight:normal\">" +
                                msg.Text + "<\span>" + "</th>" +
                                "</tr>"
                            );
                        }
                    }
                }
            }
        }
    });
}

function getCID(data){
    if (data.length > 0) {
        document.getElementById("knownCIDs").style.visibility = "visible";
        var list = document.getElementById("cIDs")
        var oldChildStyle = {}
        while (list.hasChildNodes()) {
            var oldChild = list.removeChild(list.lastChild);
            oldChildStyle[oldChild.innerHTML] = oldChild.style
        }
        
        for(const x of data) {
                $("#cIDs").append(
                    "<li id=\"cidentifier_" + x + "\"" + ">" +
                    x +
                    "</li>"
                );
                if (oldChildStyle[x] != undefined) {
                    document.getElementById("cidentifier_" + x).style.color = oldChildStyle[x].color
                    document.getElementById("cidentifier_" + x).style.fontWeight = oldChildStyle[x].fontWeight
                } else {
                    createTableIfNotExist(x)
                }
                $("#cidentifier_" + x).click(function () {
                    if (cID === x.toString()) {
                        // go back to public chat
                        cID = "nil"
                        document.getElementById("cidentifier_" + x).style.fontWeight = "normal"
                        document.getElementById("chat").innerText = "Public Chat"
                        document.getElementById("sendMessage").placeholder = "Type your public message here"
    
                        /*document.getElementById("fileRequest").placeholder = "Please first select a node for the request"
                        document.getElementById("hashRequest").placeholder = "Please first select a node for the request"*/
    
                        document.getElementById("private_" + x + "_messages").style.visibility = "collapse"
                        document.getElementById("public_messages").style.visibility = "visible"
                    } else if (cID === "nil") {
                        // change red color that notified new messages if necessary
                        document.getElementById("cidentifier_" + x).style.color = "black"
    
                        cID = x.toString()
                        document.getElementById("cidentifier_" + x).style.fontWeight = "bold"
                        document.getElementById("chat").innerHTML = "Private Chat" + "<br/>" + x
                        document.getElementById("sendMessage").placeholder = "Type your private message for " + cID + " here"
    
                        /*document.getElementById("fileRequest").placeholder = "Type the name of the downloaded file"
                        document.getElementById("hashRequest").placeholder = "Type metahash of the file request for " + cID + " here"*/
    
                        document.getElementById("public_messages").style.visibility = "collapse"
                        document.getElementById("private_" + x + "_messages").style.visibility = "visible"
                    }
                });
            }

    } 
}

function getTorPrivateMessages() {
    $.ajax({
        type: "GET",
        url: "http://localhost:8080/privateTor",
        dataType: 'json',
        success: function (data, status, xhr) {
            if (data != undefined) {
                //console.log(data)
                var keys = Object.keys(data)
                getCID(keys)
                for (const x of keys) {
                    if (x != cID && data[x].length > 0) {
                        document.getElementById("cidentifier_" + x).style.color = "red"
                        document.getElementById("cidentifier_" + x).style.fontWeight = "bold"
                    }
                    for (let msg of data[x]) {
                        if (verifyMessage(msg.Text)) {
                            $("#private_" + x + "_messages").find("tbody").append(
                                "<tr>" + "<th> From " + "<span style=\"font-weight:normal\">" +
                                msg.Origin + "<\span>" + "</th>" +
                                "<th> Message " + "<span style=\"font-weight:normal\">" +
                                msg.Text + "<\span>" + "</th>" +
                                "</tr>"
                            );
                        }
                    }
                }
            }
        }
    });
}

$.ajax({
    type: "GET",
    url: "http://localhost:8080/id",
    success: function (data, status, xhr) {
        var name = JSON.parse(data);
        document.getElementById("nodeName").innerHTML = name.toString()
    }
});

/****************************** POST ******************************/

// Add peer
function addNode() {
    var newNode = document.getElementById("sendNode").value;
    if (verifyIpAndPort(newNode)) {
        $.ajax({
            type: "POST",
            url: "http://localhost:8080/node",
            data: {
                "value": newNode
            },
            statusCode: {
                401: function (data, textStatus, xhr) {
                    document.getElementById("sendNode").value = '';
                    alert(data.responseText)
                }
            },
            success: function (data, status, xhr) {
                document.getElementById("sendNode").value = '';
            }
        })
    } else {
        document.getElementById("sendNode").value = '';
        alert("Invalid IP:Port !")
    }
}

function sendMessage() {
    var newMessage = document.getElementById("sendMessage").value;
    // Get anonymity
    var checkBox = document.getElementById("anonymityCheck");
    var messageType = "normal"
    if (checkBox.checked == true) {
        messageType = "anonyme"
    }
    if (newMessage != "") {
        $.ajax({
            type: "POST",
            url: "http://localhost:8080/message",
            data: {
                "value": newMessage,
                "identifier": currentChat,
                "messagetype": messageType,
                "cid": cID
            },
            success: function (data, status, xhr) {
                document.getElementById("sendMessage").value = '';
            }
        })
    } else {
        alert("Can't send empty message !")
    }
}

function addrRequest() {
    var onionAddr = document.getElementById("onionAddrRequest").value;
    if (onionAddr != "") {
        $.ajax({
            type: "POST",
            url: "http://localhost:8080/onionAddr",
            data: {
                "value": onionAddr
            },
            success: function (data, status, xhr) {
                document.getElementById("onionAddrRequest").value = '';
            }
        })
    } else {
        alert("Enter the desired Onion Address!")
    }
}

function getHTML() {
    $.ajax({
        type: "GET",
        url: "http://localhost:8080/htmlGetter",
        dataType: 'json',
        success: function (data, status, xhr) {
            if (data != undefined) {
                //console.log(data)
                var keys = Object.keys(data)
                for (const x of keys) {
                    console.log(data[x])
                    document.documentElement.innerHTML = data[x]
                }
            }
        }
    });
}

// Send an index file request
/*function sendIndex() {
    var fileName = document.getElementById("fileIndex").files[0].name;
    if (fileName != "") {
        $.ajax({
            type: "POST",
            url: "http://localhost:8080/file",
            data: {
                "value": fileName,
                "identifier": "public"
            },
            success: function (data, status, xhr) {
                document.getElementById("fileIndex").value = '';
            }
        })
    } else {
        alert("Can't index file !")
    }
}*/

// Send a file download request
/*function sendDownload() {
    var fileName = document.getElementById("fileRequest").value;
    var requestHash = document.getElementById("hashRequest").value;
    if (fileName != "" && requestHash != "" && currentChat != "public") {
        $.ajax({
            type: "POST",
            url: "http://localhost:8080/file",
            data: {
                "value": fileName,
                "request": requestHash,
                "identifier": currentChat
            },
            statusCode: {
                401: function (data, textStatus, xhr) {
                    document.getElementById("fileRequest").value = '';
                    document.getElementById("hashRequest").value = '';
                    alert(data.responseText)
                }
            },
            success: function (data, status, xhr) {
                document.getElementById("fileRequest").value = '';
                document.getElementById("hashRequest").value = '';
            }
        })
    } else if(currentChat == "public") {
        alert("Please first select a node for the request !")
    } else {  
        alert("Must specify a filename and a request metahash !")
    }
}*/

// Send a search file request
/*function sendSearch() {
    var searchKeywords = document.getElementById("searchRequest").value;
    var searchBudget = document.getElementById("budgetRequest").value;
    if (searchKeywords != "") {
        $.ajax({
            type: "POST",
            url: "http://localhost:8080/search",
            data: {
                "value": searchKeywords,
                "budget": searchBudget,
            },
            statusCode: {
                401: function (data, textStatus, xhr) {
                    document.getElementById("searchRequest").value = '';
                    document.getElementById("budgetRequest").value = '';
                    alert(data.responseText)
                }
            },
            success: function (data, status, xhr) {
                document.getElementById("searchRequest").value = '';
                document.getElementById("budgetRequest").value = '';
            }
        })
    } else {
        document.getElementById("budgetRequest").value = '';
        alert("Please specify at least one keyword for the search !")
    }
}*/

/****************************** INIT ******************************/
setInterval(getMessages, 700);
setInterval(getPeers, 700);
setInterval(getPrivateMessages, 700);
setInterval(getTorPrivateMessages, 700);
setInterval(getHTML, 700);

getPeers()
getMessages()
getPrivateMessages()
getTorPrivateMessages()

/****************************** UTIL ******************************/
function validateNum(input, min, max) {
    var num = +input;
    return num >= min && num <= max && input === num.toString();
}

function verifyIPaddress(ipaddress) {
    return /^(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)$/.test(ipaddress)
}

function verifyIpAndPort(input) {
    var parts = input.split(":")
    var ip = parts[0]
    var port = parts[1]
    return validateNum(port, 1, 65535) && verifyIPaddress(ip)
}

function verifyMessage(message) {
    let test = $.parseHTML(message)
    if (test[0] != undefined) {
        return $.parseHTML(message)[0]["textContent"] === message
    }
    return false
}