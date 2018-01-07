(function(){
    
    String.prototype.replaceAll = function(search, replacement) {
        var target = this;
        return target.split(search).join(replacement);
    };
    
    var Editor = function(app){

        var _this = this;
        
        this.app = app;
        this.ace;
        
        this.dom = document.querySelector('.code-editor');
        

        /// TODO: temp fix missing semicolon warning (annoying)
        
        this.init = function(){

            this.ace = ace.edit("editor");
            var editor = this.ace;
            
            editor.$blockScrolling = Infinity

            editor.setTheme("ace/theme/crimson_editor");
            editor.getSession().setMode("ace/mode/python");
            
            editor.renderer.setScrollMargin(10, 10)

            // read existing content from local storage
            var localStorageCode = window.localStorage.getItem("editor-code");
            if(localStorageCode){
                editor.setValue(localStorageCode, 1);
            }

            // init key event listeners
            editor.container.addEventListener('keydown', function(e){
                

                // save current content to localStorage
                var code = editor.getValue();
                window.localStorage.setItem("editor-code", code);
                

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

                    // I know, it is evil, but this way I'll be able to have awesome features!
                    
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
                    session.insert(editor.getCursorPosition(), e)
                }
            }

        }

        this.evalScript = function(script){
            if(script.trim().length > 0){
                this.app.wsManager.send("#PARSE#" + script);
            }
        }
    }
    
    window.Editor = Editor;
    
})();