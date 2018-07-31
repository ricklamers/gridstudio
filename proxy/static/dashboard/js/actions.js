$(document).ready(function(){

    function loadWorkspaces(){
        $.get("/get-workspaces", function(data){
            // console.log(data);

            var workspaceList = $('.workspace-list');
            workspaceList.html(" ");

            var root = window.location.href.split("/dashboard")[0];

            for(var x = 0; x < data.length; x++){
                var workspaceNameEscaped = data[x].name.replace("'","&#39;");

                sharedText = "no";

                if(data[x].shared == 1){
                    sharedText = "yes";
                }

                workspaceList.append(
                    "<li>" 
                    +"<div class='workspace-controls'><form action='/initialize' method='post'><input type='hidden' value='"+data[x].slug+"' name='uuid' /><input type='hidden' value='"+data[x].id+"' name='id' /><button class='highlight'>Open</button></form><form action='/copy/"+data[x].slug+"' method='post'><button>Copy</button></form><form action='/remove/"+data[x].slug+"' method='post'><button>Remove</button></form><button class='interactive sharing' data-shared='"+data[x].shared+"'>Shared: " + sharedText + "</button></div>" +
                    "<input type='name' name='workspaceName' value='"+workspaceNameEscaped+"' /><span class='last-edited'>Created: "+data[x].created+"</span><br><span class='slug'>Share link: "+root + "/copy/" + data[x].slug+"</span> </li>");
            }

        })
    }

    loadWorkspaces();

    if(findGetParameter('error') != null){
        $('body').append("<div class='notification error'><div class='message'>" + findGetParameter('error') + "</div><div class='close'><button class='interactive'>Close</button></div></div>");
    }

    $(document).on('click', '.notification .close', function(){
        $(this).parents('.notification').remove();
    })

    function findGetParameter(parameterName) {
        var result = null,
            tmp = [];
        var items = location.search.substr(1).split("&");
        for (var index = 0; index < items.length; index++) {
            tmp = items[index].split("=");
            if (tmp[0] === parameterName) result = decodeURIComponent(tmp[1]);
        }
        return result;
    }
    
    $(document).on("click",".workspace-list li button.sharing",function(){

        var shared = 1;
        if($(this).attr('data-shared') == '1'){
            shared = 0;
        }

        var id = $(this).parent().find("input[name='id']").val();

        $.post("/workspace-change-share", {workspaceId: id, shared: shared}, function(data, error){
            if(error != "success"){
                console.error(error);
            }
            loadWorkspaces();
        })

    });


    $(document).on("change",".workspace-list li input[name=workspaceName]",function(){

        var val = $(this).val();
        var id = $(this).parent().find("input[name='id']").val();

        $.post("/workspace-change-name", {workspaceId: id, workspaceNewName: val }, function(data, error){
            if(error != "success"){
                console.error(error);
            }
        })

    });

    // only allow button to be clicked once
    var hasClickedButton = false;

    $(document).on('click' , 'button' , function(e){
        if(!$(this).hasClass('interactive')){
            if(hasClickedButton) {
                alert("Please wait a moment, the workspace is loading.")
                e.preventDefault();
                return false;
            }
            hasClickedButton = true;
        }
    })
});