(function(){
    
    String.prototype.replaceAll = function(search, replacement) {
        var target = this;
        return target.split(search).join(replacement);
    };
    
    var Editor = function(app){

        var _this = this;
        
        this.app = app;
        this.ace;
        this.filepath;

        this.dom = $('.code-editor');
        

        /// TODO: temp fix missing semicolon warning (annoying)
        
        this.init = function(){

            this.ace = ace.edit("editor-ace");
            var editor = this.ace;
            
            editor.$blockScrolling = Infinity

            this.dom.find('.close').click(function(){
                _this.ace.setValue("",-1);
                _this.setFilePath();
                _this.dom.find(".file-name").removeClass("unsaved");
            });

            editor.setTheme("ace/theme/crimson_editor");
            editor.getSession().setMode("ace/mode/python");

            editor.getSession().on('change', function() {
                _this.dom.find(".file-name").addClass("unsaved");
            });
            
            editor.renderer.setScrollMargin(10, 10)

            // read existing content from local storage
            // var localStorageCode = window.localStorage.getItem("editor-code");
            // if(localStorageCode){
            //     editor.setValue(localStorageCode, 1);
            // }

            // init key event listeners
            editor.container.addEventListener('keydown', function(e){
                
                if(e.keyCode == 83 && (e.ctrlKey || e.metaKey)){
                    e.preventDefault();

                    _this.saveFile();
                }

                if(e.keyCode == 13 && (e.ctrlKey || e.metaKey)){
                    
                    // get contents from current line
                    var selectionRange = editor.getSelectionRange();

                    var startRow = selectionRange.start.row;
                    var endRow = selectionRange.end.row;

                    if(startRow == endRow){

                        // single mode
                        var current_line = selectionRange.start.row;
                        var script = editor.session.getLine(current_line);


                    }else{
                        // selection mode, execute all in selection
                        var script = editor.session.getTextRange(selectionRange);

                    }

                    // evalScript per line
                    // var scriptLines = script.split("\n");
                    // for(var x = 0; x < scriptLines.length; x++){
                    //     _this.evalScript(scriptLines[x]);
                    // }
                    _this.evalScript(script);

                    var range = editor.getSelectionRange();

                    // only jump if not selection
                    if(range.start.row == range.end.row && range.start.column == range.end.column){
                        // push cursor to next line
                        var session = editor.session;
                        if(session.getLength()-1 == endRow){
                            // insert newline at end if no line at the bottom
                            session.insert({
                                row: session.getLength(),
                                column: 0
                            }, "\n")
                        }

                        editor.gotoLine(endRow + 2, 0)
                    }

                }

            })

            editor.onPaste = function(e){
                if(editor.isFocused()){
                    var session = editor.session;
                    session.remove(session.selection);
                    session.insert(editor.getCursorPosition(), e)
                }
            }

            var animationDiv = $(document.createElement("div"));
            animationDiv.addClass('computing-indicator');
            animationDiv.attr("title","Computing, please be patient.");
            animationDiv.hide();
            $('.file-editor').prepend(animationDiv);

        }

        this.setFilePath = function(path){
            this.filepath = path;

            if(!path){
                this.dom.find('.file-name').html("");
            }else{
                this.dom.find('.file-name').html(path);
            }
        }

        this.saveFile = function(){
            if(!this.filepath){
                var filename = prompt("Enter a filename");
                this.setFilePath( "/home/user/" + filename);
            }
            var content = this.ace.getValue();

            this.app.wsManager.send(JSON.stringify({arguments: ["SET-FILE", this.filepath, content]}));
            this.dom.find(".file-name").removeClass("unsaved");

            this.app.fileManager.refresh();
        }

        this.setContent = function(data, extension, path){
            this.setFilePath(path);

            if(extension == "py"){
                this.ace.getSession().setMode("ace/mode/python");
            }else{
                this.ace.getSession().setMode("ace/mode/text");
            }
            this.ace.setValue(data,-1);

            _this.dom.find(".file-name").removeClass("unsaved");
        }
        this.hideScriptExecuting = function(){
            $('#editor .computing-indicator').hide();
        }

        this.showScriptExecuting = function(){
            $('#editor .computing-indicator').show();
        }

        this.commandComplete = function(){
            this.hideScriptExecuting();
        }

        this.evalScript = function(script){
            var parsedScript = script.trim();

            parsedScript = parsedScript.replace(/^\s*\n/gm, '');

            console.log(parsedScript);

            if(parsedScript.length > 0){
                this.showScriptExecuting();
                this.app.wsManager.send("#PARSE#" + parsedScript);
            }
        }
    }
    
    window.Editor = Editor;
    
})();