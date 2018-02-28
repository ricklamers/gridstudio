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
        this.uiInit();
        this.createTerminal();
      }
            
      this.isFocused = function(){
        return this.term.isFocused;
      }
    
      this.showTab = function(selector){

        $('.dev-tabs .tab[data-tab="'+selector+'"]').addClass('current').siblings().removeClass('current');

        // hide both
        $('.dev-tabs .terminal, .dev-tabs .console').hide();
    
        // show selected
        $('.dev-tabs .' + selector).show();
      }
    
      this.uiInit = function(){
        // Tabbed area
        $('.dev-tabs .tab').click(function(){

          var selector = $(this).attr('data-tab');
          _this.showTab(selector);
          
        });
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
    
          fetch(url, {method: 'POST'});
        });
        
        protocol = (location.protocol === 'https:') ? 'wss://' : 'ws://';
        var port = 3000;
    
        // get port cookie
        function getCookie(name) {
            var value = "; " + document.cookie;
            var parts = value.split("; " + name + "=");
            if (parts.length == 2) return parts.pop().split(";").shift();
        }
        port = getCookie("term_port");
    
        socketURL = protocol + location.hostname + ((port) ? (':' + port) : '') + '/terminals/';
        var fetchUrl = 'http://' + location.hostname + ((port) ? (':' + port) : '');
    
        term.open(terminalContainer);
        term.winptyCompatInit();
        term.fit();
        term.focus();
    
        var paramFetchUrl = fetchUrl+'/terminals?cols=' + term.cols + '&rows=' + term.rows;
    
        fetch(paramFetchUrl, {method: 'POST'}).then(function (res) {
    
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


