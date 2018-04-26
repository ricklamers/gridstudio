$(document).ready(function(){

    function loadWorkspaces(){
        $.get("/get-workspaces", function(data){
            // console.log(data);

            var workspaceList = $('.workspace-list');
            workspaceList.html(" ");

            for(var x = 0; x < data.length; x++){
                workspaceList.append("<li><form action='/initialize' method='post'><input type='hidden' value='"+data[x].slug+"' name='uuid' />"+data[x].slug+"<button>Open</button></form></li><br>");
            }

        })
    }

    loadWorkspaces();

});