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

        this.runCurrentSelection = function(){
            var editor = this.ace;
            
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
        
        this.init = function(){

            this.ace = ace.edit("editor-ace");
            var editor = this.ace;
            
            editor.$blockScrolling = Infinity

            this.dom.find('.close').click(function(){
                _this.ace.setValue("",-1);
                _this.setFilePath();
                _this.dom.find(".file-name").removeClass("unsaved");
                _this.dom.find(".close").hide();
            });

            editor.setTheme("ace/theme/crimson_editor");
            editor.getSession().setMode("ace/mode/python");

            editor.getSession().on('change', function() {
                _this.dom.find(".file-name").addClass("unsaved");
                _this.dom.find(".close").show();
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
                    
                    _this.runCurrentSelection();

                }

            })

            editor.onPaste = function(e){
                if(editor.isFocused()){
                    var session = editor.session;
                    if(editor.getSelectedText().length != 0){
                        var range = editor.selection.getRange();
                        session.replace(range, '')
                    }
                    session.insert(editor.getCursorPosition(), e)
                }
            }

            var animationDiv = $(document.createElement("div"));
            animationDiv.addClass('computing-indicator');
            animationDiv.attr("title","Computing, please be patient.");
            animationDiv.hide();

            var runCode = $(document.createElement("div"));
            runCode.addClass('run-code-button');
            runCode.attr("title", "You can also use Ctrl/Command + Enter to execute the current line or selection (if you have selected anything).")

            runCode.click(function(){
                _this.runCurrentSelection();
                editor.focus();
            })

            var editorActionHolder = $(document.createElement('div'));
            editorActionHolder.addClass('editor-action-holder');
            editorActionHolder.append(animationDiv);
            editorActionHolder.append(runCode);

            $('.file-editor').prepend(editorActionHolder);

        }

        this.setFilePath = function(path){
            this.filepath = path;

            if(!path){
                this.dom.find('.file-name').html("");
            }else{
                this.dom.find('.file-name').html(path);
            }
        }

        this.insertAfterCursorLine = function(code){
            var editor = this.ace;

            selectionRange = editor.getSelectionRange();

            startLine = selectionRange.start.row;
            endLine = selectionRange.end.row;

            var customPosition = {
                row: endLine,
                column: 0
            };

            if(selectionRange.start.column != 0){
                code = "\n" + code
            }

            editor.session.insert(customPosition, code + "\n");

            editor.selection.setRange(selectionRange);
        }

        this.saveFile = function(){
            if(!this.filepath){
                var filename = prompt("Enter a filename");
                if(filename === null){
                    return;
                }
                this.setFilePath( this.app.fileManager.base_cwd +"/"+ filename);

                var extension = filename.split(".")[filename.split(".").length-1];

                if(extension == "py"){
                    this.ace.getSession().setMode("ace/mode/python");
                }
            }
            var content = this.ace.getValue();

            this.app.wsManager.send({arguments: ["SET-FILE", this.filepath, content]});
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
            $('.run-code-button').show();
        }

        this.showScriptExecuting = function(){
            $('#editor .computing-indicator').show();
            $('.run-code-button').hide();
        }

        this.commandComplete = function(){
            this.hideScriptExecuting();
        }

        this.evalScript = function(script){
            var parsedScript = script.trim();

            parsedScript = parsedScript.replace(/^\s*\n/gm, '');

            if(parsedScript.length > 0){
                this.showScriptExecuting();
                this.app.wsManager.send("#PARSE#" + parsedScript);
                this.app.markUnsaved();
            }
        }
    }
    
    window.Editor = Editor;
    
})();