$(document).ready(function(){

    function loadWorkspaces(){
        $.get("/get-workspaces", function(data){
            // console.log(data);

            var workspaceList = $('.workspace-list');
            workspaceList.html(" ");

            for(var x = 0; x < data.length; x++){
                var workspaceNameEscaped = data[x].name.replace("'","&#39;");
                workspaceList.append("<li><input type='name' name='workspaceName' value='"+workspaceNameEscaped+"' /><form action='/initialize' method='post'><input type='hidden' value='"+data[x].slug+"' name='uuid' /><input type='hidden' value='"+data[x].id+"' name='id' />"+data[x].slug+"<button>Open</button></form></li><br>");
            }

        })
    }

    loadWorkspaces();

    $(document).on("change",".workspace-list li input[name=workspaceName]",function(){

        var val = $(this).val();
        var id = $(this).parent().find("input[name='id']").val();

        $.post("/workspace-change-name", {workspaceId: id, workspaceNewName: val }, function(data, error){
            if(error != "success"){
                console.error(error);
            }
        })

    });
});