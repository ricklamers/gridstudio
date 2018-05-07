(function(){
    
    function escapeHtml(unsafe) {
        return unsafe
             .replace(/&/g, "&amp;")
             .replace(/</g, "&lt;")
             .replace(/>/g, "&gt;")
             .replace(/"/g, "&quot;")
             .replace(/'/g, "&#039;");
    }

    function getCookie(name) {
        var value = "; " + document.cookie;
        var parts = value.split("; " + name + "=");
        if (parts.length == 2) return parts.pop().split(";").shift();
    }
     
    function WSManager(app){
        var _this = this;
        
        this.app = app;

        this.init = function(){
            
            console.log("WSManager initialized.");

            var hostname = window.location.hostname;
            
            var wsPort = 443;
            this.ws = new WebSocket("ws:"+hostname+":" + wsPort + location.pathname + "ws");
            
            this.ws.onopen = function (event) {
                // _this.ws.send("Send smth!"); 

                if(_this.onconnect){
                    _this.onconnect();
                }
            };
            
            this.ws.onmessage = function (event) {
                
                var lines = event.data;
    
                lines = lines.split("\n");
    
                // DEBUGINFO
                // if(lines.length > 1){
                //     console.warn("Got back multiple lines in one onmessage event");
                //     console.log(lines);
                // }

                for(var x = 0; x < lines.length; x++){

                    // check for empty trailing newlines in websocket messages
                    if(lines[x].length > 0){

                        var json = JSON.parse(lines[x]);

                        if (json[0] == 'SET'){
            
                            for(var x = 1; x < json.length; x += 3){
                                var rowText = json[x].replace(/^\D+/g, '');
                                var rowNumber = parseInt(rowText)-1;
                
                                var columnText = json[x].replace(rowText, '');
                                var columnNumber = _this.app.lettersToIndex(columnText)-1;
                
                                var position = [rowNumber, columnNumber];
                                _this.app.set(position,json[x+1]);
                                
                                // make sure to not trigger a re-send
                                // filter empty response
                                _this.app.set_formula(position, json[x+2], false);
                            }
                        }
                        else if(json[0] == "INTERPRETER"){
                            var consoleText = json[1];
                            consoleText = escapeHtml(consoleText);
                            consoleText = consoleText.replaceAll("\n", "<br>");

                            if(consoleText.indexOf("[error]") != -1){
                                consoleText = consoleText.replace("[error]","");
                                _this.app.console.append("<div class='message error'>" + consoleText + "</div>");
                            }else{
                                _this.app.console.append("<div class='message'>" + consoleText + "</div>");
                            }
                            _this.app.console[0].scrollTop = _this.app.console[0].scrollHeight;

                            _this.app.termManager.showTab("console");
                        }else if(json[0] == "SHEETSIZE"){
                            _this.app.setSheetSize(parseInt(json[1]),parseInt(json[2]));
                        }
                        else if(json[0] == "SAVED"){
                			alert("Saved workspace");
                        }
                        else if(json.arguments && json.arguments[0] == "IMAGE"){
                            
                            var img = document.createElement('img');
                            img.setAttribute("title","Click to enlarge");
                            img.src = "data:image/png;base64, " + json.arguments[1];
                            $(".dev-tabs .plots").html(img);

                            _this.app.termManager.showTab("plots");
                            
                        }
                        else if(json[0] == "EXPORT-CSV"){
                            download(json[1],"sheet.csv");
                        }
                        else{
                            console.warn("Received WS message without case: ")
                            console.warn(json)
                        }

                    }
                    
                }
                
                // re-render on receive data
                _this.app.drawSheet();
                
                // re-render plots
                _this.app.update_plots();
    
            };
        }

        this.send = function(value){
            if(this.ws.readyState == this.ws.OPEN){
                this.ws.send(value);
            }else{
                console.warn("Tried to send" + value + " while not in open state");
            }
        }

    }

    function download(data, filename, type) {
        var file = new Blob([data], {type: type});
        if (window.navigator.msSaveOrOpenBlob) // IE10+
            window.navigator.msSaveOrOpenBlob(file, filename);
        else { // Others
            var a = document.createElement("a"),
                    url = URL.createObjectURL(file);
            a.href = url;
            a.download = filename;
            document.body.appendChild(a);
            a.click();
            setTimeout(function() {
                document.body.removeChild(a);
                window.URL.revokeObjectURL(url);  
            }, 0); 
        }
    }

    window.WSManager = WSManager;
})()
