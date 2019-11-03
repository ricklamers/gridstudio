(function(){
    
    function escapeHtml(unsafe) {
        return unsafe
             .replace(/&/g, "&amp;")
             .replace(/</g, "&lt;")
             .replace(/>/g, "&gt;")
             .replace(/"/g, "&quot;")
             .replace(/'/g, "&#039;");
    }

    function WSManager(app){
        var _this = this;
        
        this.app = app;

        this.init = function(){
            
            console.log("WSManager initialized.");

            // use host from current url
            var host = window.location.host;
            
            this.ws = new WebSocket("ws:"+ host + location.pathname + "ws");
            
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
            
                            for(var i = 1; i < json.length; i += 4){
                                var rowText = json[i].replace(/^\D+/g, '');
                                var rowNumber = parseInt(rowText)-1;
                
                                var columnText = json[i].replace(rowText, '');
                                var columnNumber = _this.app.lettersToIndex(columnText)-1;
                
                                var position = [rowNumber, columnNumber];
                                _this.app.set(position,json[i+1], parseInt(json[i+3]));
                                
                                // make sure to not trigger a re-send
                                // filter empty response
                                _this.app.set_formula(position, json[i+2], false, parseInt(json[i+3]));
                            }

                            // re-render plots
                            _this.app.update_plots();
                            
                            // re-render on SET data
                            _this.app.drawSheet();
                        }
                        else if(json[0] == "SETSHEETS"){
                        
                            // each element contains: "name", "rowCount", "columnCount"
                            json.splice(0,1);
                            _this.app.setSheets(json);

                        }
                        else if(json[0] == "INTERPRETER"){
                            var consoleText = json[1];
                            consoleText = escapeHtml(consoleText);

                            if(consoleText.indexOf("[error]") != -1){
                                consoleText = consoleText.replace("[error]","");
                                _this.app.console.append("<div class='message error'>" + consoleText + "</div>");
                            }else{
                                _this.app.console.append("<div class='message'>" + consoleText + "</div>");
                            }
                            _this.app.console[0].scrollTop = _this.app.console[0].scrollHeight;

                            _this.app.showTab("console");
                            
                        }else if(json[0] == "SHEETSIZE"){

                            var rowCount = parseInt(json[1]);
                            var columnCount = parseInt(json[2]);
                            var sheetIndex = parseInt(json[3]);
                            
                            _this.app.updateSheetSize(rowCount, columnCount, sheetIndex);
                            _this.app.drawSheet();

                            
                        }
                        else if(json[0] == "SAVED"){
                            _this.app.markSaved();
                        }
                        else if(json[0] == "PROGRESSINDICATOR"){

                            var progress = json[1];
                            if(progress == 1){
                                $('.progress-indicator-inner').css({width: 0})
                            }else{
                                $('.progress-indicator-inner').css({width: progress*100 + "%"})
                            }

                        }
                        else if(json[0] == "VIEW-INVALIDATED"){
                			_this.app.refreshView();
                        }
                        else if(json[0] == "COMMANDCOMPLETE"){
                			_this.app.editor.commandComplete();
                        }
                        else if(json[0] == "GET-DIRECTORY"){
                            json.splice(0,1);
                			_this.app.fileManager.showDirectory(json);
                        }
                        else if(json[0] == "GET-FILE"){

                            var fileDecoded = b64DecodeUnicode(json[2]);
                            var fileSplit = json[1].split(".");
                            var extension = fileSplit[fileSplit.length -1];
                            _this.app.editor.setContent(fileDecoded, extension, json[1]);

                        }
                        else if(json[0] == "JUMPCELL"){

                            if(_this.app.callbacks.jumpCellCallback){

                                var cellReference = json[3];
                                var cell = _this.app.referenceToZeroIndexedArray(cellReference);
                                _this.app.callbacks.jumpCellCallback(cell);
                            }

                        }
                        else if(json[0] == "MAXCOLUMNWIDTH"){

                            var rowIndex = parseInt(json[1]) - 1;
                            var columnIndex = parseInt(json[2]) - 1;
                            var sheetIndex = parseInt(json[3]);
                            var maxLength = parseInt(json[4]);
                            
                            var cell = [rowIndex, columnIndex];

                            var measuredWidth = _this.app.cellWidth;

                            if(maxLength != 0){
                                measuredWidth = _this.app.computeCellTextSize(_this.app.get(cell, sheetIndex)) + _this.app.textPadding*2;
                                if (measuredWidth < _this.app.minColRowSize) {
                                    measuredWidth = _this.app.minColRowSize;
                                }
                            }

                            _this.app.columnWidths(columnIndex, measuredWidth);

                            // re-render on columnWidths modification
                            _this.app.drawSheet();

                        }
                        else if(json.arguments && json.arguments[0] == "IMAGE"){
                            
                            var img = document.createElement('img');
                            img.setAttribute("title","Click to enlarge");
                            img.src = "data:image/svg+xml;base64, " + json.arguments[1];

                            _this.app.addStaticPlot(img);

                            _this.app.showTab("plots");
                            
                        }
                        else if(json[0] == "EXPORT-CSV"){
                            download(json[1],"sheet.csv");
                        }
                        else if(json[0] == "TESTCALLBACK-PONG"){
                            _this.app.testManager.currentTestCallback.apply(_this.app.testManager);                            
                        }
                        else{
                            console.warn("Received WS message without case: ")
                            console.warn(json)
                        }

                    }
                    
                }
                
                
    
            };
        }

        this.send = function(value){
            if(this.ws.readyState == this.ws.OPEN){

                // only allow non string types
                if(typeof value == "string"){
                    this.ws.send(value);
                }else {
                    
                    const noEditActions = ["GET", "JUMPCELL", "MAXCOLUMNWIDTH", "SWITCHSHEET", "GET-FILE", "SEND-FILE", "GET-DIRECTORY"];

                    if(noEditActions.indexOf(value.arguments[0]) === -1){
                        this.app.markUnsaved();
                    }

                    this.ws.send(JSON.stringify(value))
                }
            }else{
                console.warn("Tried to send" + value + " while not in open state");
            }
        }

    }

    function b64EncodeUnicode(str) {
        // first we use encodeURIComponent to get percent-encoded UTF-8,
        // then we convert the percent encodings into raw bytes which
        // can be fed into btoa.
        return btoa(encodeURIComponent(str).replace(/%([0-9A-F]{2})/g,
            function toSolidBytes(match, p1) {
                return String.fromCharCode('0x' + p1);
        }));
    }

    function b64DecodeUnicode(str) {
        // Going backwards: from bytestream, to percent-encoding, to original string.
        return decodeURIComponent(atob(str).split('').map(function(c) {
            return '%' + ('00' + c.charCodeAt(0).toString(16)).slice(-2);
        }).join(''));
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
