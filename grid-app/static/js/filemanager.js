(function(){
    
    function FileManager(app){

        var _this = this;
        
        this.app = app;

        this.cwd = '';

        this.iconMap = {
            "r":"R",
            "apple":"apple",
            "asm":"asm",
            "mp3":"audio",
            "m4a":"audio",
            "ogg":"audio",
            "babel":"babel",
            "bower":"bower",
            "bsl":"bsl",
            "cs":"c-sharp",
            "c":"c",
            "cake":"cake",
            "ctp":"cake_php",
            "cjsx":"cjsx",
            "clj":"clojure",
            "codeclimate":"code-climate",
            "coffee":"coffee",
            "erb":"coffee_erb",
            "cfc":"coldfusion",
            "cfm":"coldfusion",
            "cpp":"cpp",
            "rpt":"crystal",
            "rpte":"crystal_embedded",
            "css":"css",
            "csv":"csv",
            "d":"d",
            "db":"db",
            "dockerfile":"docker",
            "ejs":"ejs",
            "ex":"elixir",
            "exs":"elixir_script",
            "elm":"elm",
            "eslintrc":"eslint",
            "sol":"ethereum",
            "fs":"f-sharp",
            "firebase":"firebase",
            "firefox":"firefox",
            "ws":"font",
            "ttf":"font",
            "git":"git",
            "gitignore":"git_ignore",
            "github":"github",
            "go":"go",
            "gradle":"gradle",
            "grails":"grails",
            "grunt":"grunt",
            "gulp":"gulp",
            "hack":"hacklang",
            "haml":"haml",
            "hs":"haskell",
            "lhs":"haskell",
            "hx":"haxe",
            "heroku":"heroku",
            "hex":"hex",
            "html":"html",
            "html.erb":"html_erb",
            "ai":"illustrator",
            "jpg":"image",
            "png":"image",
            "jpeg":"image",
            "gif":"image",
            "bmp":"image",
            "jade":"jade",
            "java":"java",
            "js":"javascript",
            "jenkinsfile":"jenkins",
            "jinja":"jinja",
            "js.erb":"js_erb",
            "json":"json",
            "julia":"julia",
            "karma":"karma",
            "less":"less",
            "license":"license",
            "liquid":"liquid",
            "ls":"livescript",
            "lock":"lock",
            "lua":"lua",
            "makefile":"makefile",
            "md":"markdown",
            "maven":"maven",
            "mdo":"mdo",
            "mustache":"mustache",
            "npm":"npm",
            "nunjucks":"nunjucks",
            "ocaml":"ocaml",
            "odata":"odata",
            "pdf":"pdf",
            "perl":"perl",
            "ps":"photoshop",
            "php":"php",
            "ps1":"powershell",
            "pug":"pug",
            "puppet":"puppet",
            "py":"python",
            "rails":"rails",
            "react":"react",
            "rollup":"rollup",
            "rb":"ruby",
            "rs":"rust",
            "salesforce":"salesforce",
            "sass":"sass",
            "sbt":"sbt",
            "scala":"scala",
            "sh":"shell",
            "slim":"slim",
            "smarty":"smarty",
            "spring":"spring",
            "styles":"stylus",
            "sublime":"sublime",
            "svg":"svg",
            "swift":"swift",
            "terraform":"terraform",
            "tex":"tex",
            "timecop":"time-cop",
            "twig":"twig",
            "ts":"typescript",
            "vala":"vala",
            "mp4":"video",
            "avi":"video",
            "mov":"video",
            "vue":"vue",
            "webpack":"webpack",
            "wgt":"wgt",
            "windows":"windows",
            "docx":"word",
            "doc":"word",
            "xls":"xls",
            "xlsx":"xls",
            "xml":"xml",
            "yarn":"yarn",
            "yml":"yml",
            "zip":"zip"
        };

        this.init = function(){
            this.base_cwd = "/home/userdata/workspace-" + this.app.slug + "/userfolder";
            this.cwd = this.base_cwd;

            this.dom = $('.dev-tabs .view.filemanager');
            this.getDir(this.cwd);

            this.dom.find('.files-home').click(function(){
                _this.getDir(this.base_cwd)
            })

            this.dom.on('click','li.file,li.directory',function(e){
                var path = $(this).attr("data-path");

                if($(this).hasClass("file")){
                    _this.app.wsManager.send({arguments: ["GET-FILE", path]})
                }else if($(this).hasClass("directory")){
                    _this.getDir(path);
                }
            });

            this.dom.find(".path input").on("keydown", function(e){
                if(e.keyCode == 13){
                    var newPath = $(this).val();
                    _this.getDir(newPath);
                }
            })

            // try to get main.py and open it (if it exists)
            this.app.wsManager.send({arguments: ["GET-FILE", this.base_cwd + "/main.py"]})
        }

        this.getDir = function(path){
            this.cwd = path;
            this.dom.find(".path input").val(path);            
            this.getCwd();
        }

        this.refresh = function(){
            this.getDir(this.cwd);
        }

        this.getCwd = function(){
            this.app.wsManager.send({arguments: ["GET-DIRECTORY", this.cwd]})
        }

        this.showDirectory = function(data){

            if(data[0] == "INVALIDPATH"){
                console.log("Requested path does not exist.");
                return;
            }

            // clear directory
            this.dom.find(".files").html("");

            var html = "";

            // array will be sent in format: ["type", "name", "<path>"]
            for(var x = 0; x < data.length; x+= 3){
                var type = data[x];
                var name = data[x+1];
                var path = data[x+2];

                var icon = "<div class=\"icon\" style=\"background-image: url('image/file-icons/folder.svg')\"></div>";

                if(type == "file"){
                    var fileSplit = name.split(".");
                    var extension = fileSplit[fileSplit.length-1];
                    var svgPath = this.iconMap[extension.toLowerCase()];
                    if(!svgPath){
                        svgPath = "default";
                    }
                    var iconUrl = "image/file-icons/" + svgPath + ".svg";
                    icon = "<div class=\"icon\" style=\"background-image: url('"+iconUrl+"')\"></div>";
                }

                html += "<li class='"+type+"' data-path='"+path+"'>"+icon+name+"</li>";
            }
            
            this.dom.find(".files").html(html);
            
        }

    }

    window.FileManager = FileManager;
})()
