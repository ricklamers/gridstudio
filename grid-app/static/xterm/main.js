import * as Terminal from './xterm.js/xterm';
import * as attach from './xterm.js/addons/attach/attach';
import * as fit from './xterm.js/addons/fit/fit';
import * as fullscreen from './xterm.js/addons/fullscreen/fullscreen';
import * as search from './xterm.js/addons/search/search';
import * as winptyCompat from './xterm.js/addons/winptyCompat/winptyCompat';


Terminal.applyAddon(attach);
Terminal.applyAddon(fit);
Terminal.applyAddon(fullscreen);
Terminal.applyAddon(search);
Terminal.applyAddon(winptyCompat);

(function(){

    var TermManager = function(app){

      var terminalContainer = document.getElementById('terminal-container')
    
      var protocol,
        socketURL,
        socket,
        pid;
    
      var _this = this;
      var term;
    
      this.app = app;
    
      this.init = function(){
        this.createTerminal();
      }
            
      this.isFocused = function(){
        return this.term.isFocused;
      }
    
      this.runRealTerminal = function(){
        term.attach(socket);
        term._initialized = true;
      }
    
      this.createTerminal = function(){
    
        // Clean terminal
        while (terminalContainer.children.length) {
          terminalContainer.removeChild(terminalContainer.children[0]);
        }
        term = new Terminal({
          cursorBlink: true,
          scrollback: 1000,
          tabStopWidth: 8
        });
        this.term = term;
        
        window.term = term;  // Expose `term` to window for debugging purposes
        term.on('resize', function (size) {
          if (!pid) {
            return;
          }
          var cols = size.cols,
              rows = size.rows,
              url = '/terminals/' + pid + '/size?cols=' + cols + '&rows=' + rows;
    
          fetch(url, {method: 'POST', credentials: "same-origin"});
        });
        
        protocol = (location.protocol === 'https:') ? 'wss://' : 'ws://';
        
        // use host from current url
        var host = location.host;
    
        socketURL = protocol + host + location.pathname + 'terminals/';
        var fetchUrl = location.pathname;
    
        term.open(terminalContainer);
        term.winptyCompatInit();
        term.fit();
        // term.focus();
    
        var paramFetchUrl = fetchUrl + 'terminals?cols=' + term.cols + '&rows=' + term.rows;
    
        fetch(paramFetchUrl, {method: 'POST', credentials: "same-origin"}).then(function (res) {
    
          res.text().then(function (processId) {
            pid = processId;
            socketURL += processId;
            socket = new WebSocket(socketURL);
            socket.onopen = _this.runRealTerminal;
            socket.onclose = function(e){
              console.log(e);
            };
            socket.onerror = function(e){
              console.log(e);
            };
          });
        });

      }

   }

   window.TermManager = TermManager;

})();


