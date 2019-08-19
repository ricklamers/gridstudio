function CodeGen(app){

    this.app = app;

    this.init = function(){
    }

    this.getDataString = function(selection, sheetIndex){
        code = ""
        if(sheetIndex == 0){
            code += 'data = sheet("'+selection+'")\n\n';
        }else{
            code += 'data = sheet("'+selection+'", sheet_index = '+sheetIndex+')\n\n';
        }
        return code
    }

    this.generate = function(method, selection, sheetIndex){

        var code = "";

        switch(method){
            case "pandas-get-data":

                code += this.getDataString(selection, sheetIndex)
                code += 'print(data)';

                break;
            case "pandas-plot-data":

                code += this.getDataString(selection, sheetIndex)
                code += 'data.plot()\n';
                code += 'show()';

                break;

            case "pandas-plot-hist-data":

                code += this.getDataString(selection, sheetIndex)
                code += 'data.hist()\n';
                code += 'show()';

                break;
                
            case "pandas-average":

                code += this.getDataString(selection, sheetIndex)
                code += 'data_average = data.mean()\n';
                code += 'print(data_average)';

                break;
                
            case "pandas-std-deviation":

                code += this.getDataString(selection, sheetIndex)
                code += 'data_std_deviation = data.std()\n';
                code += 'print(data_std_deviation)';

                break;

            case "pandas-variance":

                code += this.getDataString(selection, sheetIndex)
                code += 'data_variance = data.var()\n';
                code += 'print(data_variance)';

                break;

            case "pandas-quartile-q1":

                code += this.getDataString(selection, sheetIndex)
                code += 'data_q1 = data.quantile(.25)\n';
                code += 'print(data_q1)';

                break;

            case "pandas-quartile-q3":

                code += this.getDataString(selection, sheetIndex)
                code += 'data_q3 = data.quantile(.75)\n';
                code += 'print(data_q3)';

                break;

            case "pandas-frequency-table":

                code += this.getDataString(selection, sheetIndex)
                code += 'data_value_counts = data[0].value_counts()\n';
                code += 'print(data_value_counts)';

                break;
        }

        if(code.length > 0){
            this.app.editor.insertAfterCursorLine(code);
            this.app.editor.ace.focus();
        }
    }
}

window.CodeGen = CodeGen
