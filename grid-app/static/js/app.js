(function(){
	
	// macOS swipe back prevention
	history.pushState(null, null, '');
	window.addEventListener('popstate', function(event) {
		history.pushState(null, null, '');
	});
	
	function download(data, filename, type) {
		var file = new Blob([data], {type: type});
		if (window.navigator.msSaveOrOpenBlob) // IE10+
			window.navigator.msSaveOrOpenBlob(file, filename);
		else { // Others
			var a = document.createElement("a"),
					url = URL.createObjectURL(file);
			a.href = url;
			a.download = filename;
			document.body.appendChild(a);
			a.click();
			setTimeout(function() {
				document.body.removeChild(a);
				window.URL.revokeObjectURL(url);  
			}, 0); 
		}
	}

	function getMaxOfArray(numArray) {
		return Math.max.apply(null, numArray);
	}

	function getMinOfArray(numArray) {
		return Math.min.apply(null, numArray);
	}
	
	function dataURLtoBytes(url){
		return (fetch(url)
			.then(function(res){return res.arrayBuffer();})
			.then(function(buf){return buf; })
		);
	}
	
	// function dataURLtoBytes(dataurl, filename) {
	// 	var arr = dataurl.split(','), mime = arr[0].match(/:(.*?);/)[1],
	// 		bstr = atob(arr[1]), n = bstr.length, u8arr = new Uint8Array(n);
	// 	while(n--){
	// 		u8arr[n] = bstr.charCodeAt(n);
	// 	}
	// 	return u8arr
	// }

	String.prototype.capitalize = function() {
		return this.replace(/(?:^|\s)\S/g, function(a) { return a.toUpperCase(); });
	};

	var App = function(){

		var _this = this;

		this.wsManager = new WSManager(this);
		this.editor = new Editor(this);
		this.termManager = new TermManager(this);

		
		
 		this.dom = document.querySelector('body');
		this.canvas = document.createElement('canvas');
		this.ctx = this.canvas.getContext('2d');
		this.sheetDom = document.querySelector('div-sheet');
		this.sheetSizer = this.sheetDom.querySelector('.sheet-sizer');
		this.formula_input = $(this.dom.querySelector('.formula-bar input'));
		this.mouse_down_canvas = false;
		this.console = $('.console');

		this.numRows = 10000;
		this.numColumns = 26;

		this.sheetOffsetY = 0;
		this.sheetOffsetX = 0;
		this.scrollOffsetX = 0;
		this.scrollOffsetY = 0;
		this.textPadding = 3;
		
		this.plots = {};

		this.minColRowSize = 20;

		this.sidebarSize = 25;

		this.drawRowStart = 0;
		this.drawColumnStart = 0;
		
		this.pixelRatio = window.devicePixelRatio;

		this.selectedCells = [[0,0],[0,0]];

		this.data = [];
		this.dataFormulas = [];

		this.rowHeights = [];
		this.columnWidths = [];

		this.init_input_field_backup_value = undefined;
		
		this.fontStyle = "12px Arial";
		this.fontHeight = determineFontHeight(this.fontStyle);

		this.get = function(position){

			// undefined is reserved for out of bounds
			if(position[0] < 0 || position[1] >= this.numColumns){
				return undefined;
			}
			if(position[1] < 0 || position[0] >= this.numRows){
				return undefined;
			}

			// do not return undefined for empty cells but an empty string (consistent with default cell values in backend)
			if(this.data[position[0]] === undefined){
				return ""; 
			}else{
				var value = this.data[position[0]][position[1]];
				if(value === undefined){
					value = "";
				}
				return value;
			}
		}
		this.get_range = function(cell1, cell2) {
			var data = [];

			for(var x = cell1[0]; x <= cell2[0]; x++){
				for(var y = cell1[1]; y <= cell2[1]; y++){
					data.push(this.get([x, y]));
				}
			}
			return data;
		}
		this.get_formula = function(position){
			if(this.dataFormulas[position[0]] === undefined){
				return undefined;
			}else{
				return this.dataFormulas[position[0]][position[1]];
			}
		}
		
		this.update_plots = function(){
			for(var key in this.plots){
				if(this.plots.hasOwnProperty(key)){
					this.update_plot(this.plots[key]);
				}
			}
		}

		this.indexToLetters = function(index){
			var base = 26;
			var leftOver = index;
			var columns = [];
			var buff = "";

			while(leftOver > 0) {
                var remainder = leftOver % base;
                if (remainder == 0){
                    remainder = base;
                }
				columns.unshift(remainder);
				leftOver = (leftOver - remainder) / base;
			}

			for(var x = 0; x < columns.length; x++){
				buff += String.fromCharCode(64 + columns[x]);
			}
			return buff;
		}
		
		this.lettersToIndex = function(letters){
			var index = 0;
			var columns = letters.length - 1;
			var base = 26;

			for(var x = 0; x < letters.length; x++){
				var number = letters[x].charCodeAt(0)-64;
				index += number * Math.pow(base, columns);
				columns--;
			}

			return index;
		}

		this.set_formula = function(position, value, update) {
			if(!this.dataFormulas[position[0]]){
				this.dataFormulas[position[0]] = [];
			}

			// check if value is numeric
			var isNumber = /^-?[0-9]\d*(\.\d+)?$/.test(value);
			if(isNumber){
				value = "=" + value;
			}

			this.dataFormulas[position[0]][position[1]] = value.toString();
			
			if(update !== false){
				this.wsManager.send(JSON.stringify({arguments: ["SET",this.indexToLetters(position[1]+1) + (position[0]+1), value.toString()]}));
			}
		}

		this.set = function(position, value){
			if(!this.data[position[0]]){
				this.data[position[0]] = [];
			}

			this.data[position[0]][position[1]] = value.toString();
		}

		// add test data (2nd row, 2nd to 5th column, 10,20,30,40)
		// var i = 0;
		// for(var x = 0; x < 50; x++){
		// 	for(var y = 0; y < 50; y++){
		// 		i++;
		// 		this.set([x,y],i);
		// 	}
		// }

		this.ctx.a_moveTo = function(x,y){
			_this.ctx.moveTo(x+0.5,y+0.5);
		}
		this.ctx.a_lineTo = function(x,y){
			_this.ctx.lineTo(x+0.5,y+0.5);
		}

		this.init_input = function(){

			// add input element to dom
			var input = document.createElement('input');
			$(input).addClass('input-field');
			this.input_field = $(input);
			this.input_field.hide();
			this.input_field.css({font: this.fontStyle});

			this.sheetDom.insertBefore(input, this.sheetDom.children[0]);
			// event listener
			// input.addEventListener('keypress', function(e){

			// })

		}

		this.sizeSizer = function(){
			
			this.canvas.width = this.sheetDom.clientWidth * this.pixelRatio;
			this.canvas.height = this.sheetDom.clientHeight * this.pixelRatio;
			this.canvas.style.width = this.sheetDom.clientWidth + "px";
			this.ctx.scale(this.pixelRatio,this.pixelRatio);

			// determine width based on columnWidths
			var sizerWidth = this.columnWidths.reduce(function(total, num){ return total + num; }, 0);

			// determine height based on rowHeights
			var sizerHeight = this.rowHeights.reduce(function(total, num){ return total + num; }, 0);

			this.sheetSizer.style.height = sizerHeight + "px";
			this.sheetSizer.style.width = sizerWidth + "px";
		}

		this.init = function(){
			
			// initialize editor
			this.editor.init();

			// initialize wsManager
			this.wsManager.init();

			// initialize terminal
			this.termManager.init();
			
			this.menuInit();

			// init input
			this.init_input();
			
			this.initRowCols();

			this.sheetSizer.appendChild(this.canvas);
			this.sizeSizer();

			this.drawSheet();

			this.sheetDom.addEventListener('scroll',function(e){

				_this.scrollOffsetX = _this.sheetDom.scrollLeft;
				_this.scrollOffsetY = _this.sheetDom.scrollTop;

				// draw canvas on scroll event
				_this.drawSheet();

			});
			
			// resize listener
			window.addEventListener('resize',function(){
				_this.resizeSheet();
			});
			
			this.isFocusedOnElement = function(){
				
				if(!_this.input_field.is(':focus') && !_this.formula_input.is(":focus") && !_this.editor.ace.isFocused()){
					// TEMP: for terminal
					return true;
					return false;
				}else{
					return true;
				}
			}

			// mouse down listener canvas
			this.sheetDom.addEventListener('dblclick',function(e){

				e.preventDefault();
				
				// prevent double click when clicking in plot
				if($(e.target).parents('.plot').length == 0){


					if(e.which == 1 && !_this.input_field.is(':focus')){

						
						var canvasMouseX = e.offsetX - _this.sheetDom.scrollLeft;
						var canvasMouseY = e.offsetY - _this.sheetDom.scrollTop;

						// check if dblclick location is in indicator area, if so, resize closes column to default rowheight
						if(canvasMouseX < _this.sidebarSize || canvasMouseY < _this.sidebarSize){
							
							var type = 'column';
							if( canvasMouseX < _this.sidebarSize ){
								type = 'row';
							}

							var cell = _this.positionToCellDivider(canvasMouseX, canvasMouseY, type);

							
							if(type == 'column'){

								_this.columnWidths[cell[1]] = _this.cellWidth;
								
							}else{

								_this.rowHeights[cell[0]] = _this.cellHeight;

							}

							// redraw with new columnWidht/rowHeights
							_this.computeScrollBounds();

							_this.drawSheet();

						}else{

							// if not double click on sidebar, open cell in location
							_this.show_input_field();
							
							var range = document.createRange();
							range.setStart(_this.input_field[0],0);
							range.setEnd(_this.input_field[0], 0);

						}
	
					}
				}
				
				
			});

			this.selectCell = function(cell) {
				this.selectedCells = [cell, cell];
				// also fill formulabar

				var formula_value = this.get_formula(cell);
				if(formula_value !== undefined){
					this.formula_input.val(formula_value);
				}else{
					this.formula_input.val(formula_value);		
				}
			}

			this.resizingIndicator = false;
			this.resizingIndicatorType = 'column';
			this.resizingIndicatorCell = undefined;
			this.resizingIndicatorPosition = [0,0];

			this.sheetDom.addEventListener('mousedown',function(e){

				if(e.which == 1 && !_this.input_field.is(':focus')){

					// also check for sheetSizer (for scrollbar), don't fall through to deselect_input_field
					if(e.target == _this.sheetSizer){
						_this.mouse_down_canvas = true;

						var canvasMouseX = e.offsetX - _this.sheetDom.scrollLeft;
						var canvasMouseY = e.offsetY - _this.sheetDom.scrollTop;

						// check if in indicator ranges -- resize column/rows
						if(canvasMouseX < _this.sidebarSize || canvasMouseY < _this.sidebarSize){
							_this.resizingIndicator = true;

							_this.resizingIndicatorPosition = [e.offsetX, e.offsetY];

							if(canvasMouseX < _this.sidebarSize){
								_this.resizingIndicatorType = 'row';
							}else{
								_this.resizingIndicatorType = 'column';
							}

							var cell = _this.positionToCellDivider(canvasMouseX, canvasMouseY, _this.resizingIndicatorType);
							
							_this.resizingIndicatorCell = cell;
							
							// identify which rowHeight or columnHeight should be transformed

							// cell[0] <- row
							// cell[1] <- column

						}else{
							// select cells

							var cell = _this.positionToCellLocation(canvasMouseX, canvasMouseY);

							if(e.shiftKey){
								// set both cells
								_this.selectedCells[1] = cell;
							}else{
								// set both cells
								_this.selectCell(cell);
							}
			
							// render cells
							_this.drawSheet();

						}
						
					}
					
				}else{
					// if clicked on sheetDom deselect
					if(e.target != _this.input_field[0] && _this.input_field.is(':focus')){
						_this.deselect_input_field();
					}
				}
				
			});

			// mouse move listener
			this.sheetDom.addEventListener('mousemove',function(e){

				if(_this.mouse_down_canvas){

					if(_this.resizingIndicator){
						
						var diff = [e.offsetX - _this.resizingIndicatorPosition[0], e.offsetY - _this.resizingIndicatorPosition[1]];
						
						if(_this.resizingIndicatorType == 'column'){
							// resizing column
							var index =_this.resizingIndicatorCell[1];
							_this.columnWidths[index] += diff[0];
							
							if(_this.columnWidths[index] < _this.minColRowSize){
								_this.columnWidths[index] = _this.minColRowSize;
							}
						}else{
							// resizing row
							var index =_this.resizingIndicatorCell[0];
							
							_this.rowHeights[index] += diff[1];
							
							if(_this.rowHeights[index] < _this.minColRowSize){
								_this.rowHeights[index] = _this.minColRowSize;
							}
						}

						// re-draw after resize
						_this.drawSheet();

						_this.resizingIndicatorPosition = [e.offsetX, e.offsetY];
						
					}else{

						// drag operation
					
						// set end cell selection as end cell
						var cell = _this.positionToCellLocation(e.offsetX - _this.sheetDom.scrollLeft, e.offsetY - _this.sheetDom.scrollTop);
						
						// redraw selection if new cell
						if(cell != _this.selectedCells[1]){
							_this.selectedCells[1] = cell.slice();

							_this.drawSheet();
						}

					}
					
					
				}

			});

			// mouse up listener
			document.body.addEventListener('mouseup',function(){
				_this.mouse_down_canvas = false;

				if(_this.resizingIndicator){
					_this.resizingIndicator = false;
					// recompute bounds on mouse up
					_this.computeScrollBounds();
					_this.drawSheet();
				}
				
			});

			document.body.addEventListener('paste', function(e){

				if(!_this.isFocusedOnElement()){
					_this.set_range(_this.selectedCells[0], _this.selectedCells[1], event.clipboardData.getData('Text'));
					
					// redraw
					_this.drawSheet();
				}
				
			});

			// register keyboard listener
			document.body.addEventListener('keydown',function(e){

				var keyRegistered = true;

				// escape
				if(e.keyCode == 27){

					// either back out input action or deselect
					if(_this.input_field.is(":focus")){
						
						// put back backup value back
						// _this.input_field.val(_this.init_input_field_backup_value);					
						_this.deselect_input_field(false);
					}else{
						// _this.selectedCells = undefined;
						_this.drawSheet();
					}
					
				}
				// left arrow
				else if(e.keyCode >= 37 && e.keyCode <= 40){

					if(!_this.isFocusedOnElement()){
						
						if(e.keyCode == 37){
							_this.translateSelection(-1, 0, e.shiftKey, e.ctrlKey || e.metaKey);
						}
						// up arrow
						else if(e.keyCode == 38){
							_this.translateSelection(0, -1, e.shiftKey, e.ctrlKey || e.metaKey);
						}
						// right arrow
						else if(e.keyCode == 39){
							_this.translateSelection(1, 0, e.shiftKey, e.ctrlKey || e.metaKey);
						}
						// down arrow
						else if(e.keyCode == 40){
							_this.translateSelection(0, 1, e.shiftKey, e.ctrlKey || e.metaKey);
						}
					}else{
						// fall through allow normal arrow key movement in input mode
						keyRegistered = false;
					}
				}
				else if(e.keyCode == 13){
					
					if(_this.isFocusedOnElement()){
						
						if(_this.formula_input.is(":focus")){
							var inputValue = _this.formula_input.val();
							_this.set_formula(_this.selectedCells[0], inputValue);
							_this.formula_input.blur();
						}
						else if(_this.input_field.is(":focus")){
							// defocus, e.g. submit to currently selected field
							_this.deselect_input_field(true);
	
							// set focus to next cell
							var nextCell = _this.selectedCells[0];
							nextCell[0]++;
							_this.selectCell(nextCell);
	
						}else{
							keyRegistered = false;
						}
						
					}else{
						
						_this.show_input_field();
						
					}
					
				}
				else if(e.keyCode == 9){

					if(_this.input_field.is(":focus")){
						
						_this.deselect_input_field(true);

						// set focus to next cell
						var nextCell = _this.selectedCells[0];
						nextCell[1]++;
						_this.selectCell(nextCell);

					}else{
						keyRegistered = false;
					}

				}
				else if(e.keyCode == 187 || e.keyCode == 61){
					if(!_this.isFocusedOnElement()){
						_this.show_input_field();
						_this.input_field.val("=");
					}else{
						keyRegistered = false;
					}
				}
				// delete
				else if(e.keyCode == 46 || e.keyCode == 8){
					// delete value in current cell
					if(!_this.input_field.is(":focus") && !_this.formula_input.is(':focus')){
						_this.deleteSelection();
						_this.formula_input.val('');
					}else{
						keyRegistered = false;
					}

				}
				else if(!e.ctrlKey && !e.metaKey && (e.keyCode == 32 || (e.keyCode >= 48 && e.keyCode <= 57) || (e.keyCode >= 65 && e.keyCode <= 90))){

					// any of the letters and the space
					if(!_this.isFocusedOnElement()){

						// _this.set_formula(_this.selectedCells[0], '');

						_this.show_input_field();
						// replace value in current field
						_this.input_field.val('');
					}

					// however, don't absorb
					keyRegistered = false;					
				}
				else if(
					(e.ctrlKey && (e.keyCode == 67)) ||
					(e.metaKey && (e.keyCode == 67))) {

					if(!_this.isFocusedOnElement()){
						
						// copy
						// set input with current cell's value
						_this.input_field.val(_this.get(_this.selectedCells[0]));
						_this.input_field.show();
						
						_this.input_field[0].select();
						document.execCommand("Copy");
						_this.input_field.hide();
					}else{
						keyRegistered = false;
					}
						
					
				}
				else{
					keyRegistered = false;			
				}

				if(keyRegistered){
					e.preventDefault();
				}
				// console.log(e);

			});
		}

		this.deselect_input_field = function(set_values){
			_this.input_field.blur();
			
			if(_this.selectedCells != undefined && set_values === true){
				_this.set_range(_this.selectedCells[0], _this.selectedCells[1], _this.input_field.val());
			}
			
			// clear value from input field
			_this.input_field.val('');
			_this.input_field.hide();

		}

		this.show_input_field = function(){


			// position input field
			// prefill with value in current cell
			this.input_field.val(this.get_formula(this.selectedCells[0]));

			this.init_input_field_backup_value = this.input_field.val();
			
			// first: position for what cell?
			var cellPosition = this.cellLocationToPosition(this.selectedCells[0]);

			// size this cell
			var cellWidth = this.columnWidths[this.selectedCells[0][1]]; // index 0, 1 is column
			var cellHeight = this.rowHeights[this.selectedCells[0][0]]; // index 0, 0 is row

			// special sizing due to Canvas+HTML combination
			this.input_field.css({width: cellWidth-1, height: cellHeight-1});

			// draw input at this position
			this.input_field.css({marginLeft: cellPosition[0] + 1 + this.sidebarSize, marginTop: cellPosition[1] + 1 + this.sidebarSize});
			
			this.input_field.show();
			this.input_field.focus();
			
		}

		this.deleteSelection = function(){
			var lower_upper_cells = this.selectionToLowerUpper(this.selectedCells);

			this.set_range(lower_upper_cells[0], lower_upper_cells[1], '');
		}

		this.selectionToLowerUpper = function(selectedCells){
			var cell1 = selectedCells[0].slice();
			var cell2 = selectedCells[1].slice();

			var columnBegin = cell1[1];
			var columnEnd = cell2[1];

			var rowBegin = cell1[0];
			var rowEnd = cell2[0];

			// swap cells if one is before other
			if(columnEnd < columnBegin){
				var tmp = columnBegin;
				columnBegin = columnEnd;
				columnEnd = tmp;
			}

			if(rowEnd < rowBegin){
				var tmp = rowBegin;
				rowBegin = rowEnd;
				rowEnd = tmp;
			}



			return [[rowBegin, columnBegin], [rowEnd,columnEnd]];
		}

		// signature (requires lower cell, to higher cell)
		this.set_range = function(cell, ending_cell, value){

			// delete range
			var cellRangeSize = this.cellRangeSize(cell, ending_cell);
			for(var x = 0; x < cellRangeSize[0]; x++){
				for(var y = 0; y < cellRangeSize[1]; y++){
					var current_cell = [cell[0] + y, cell[1] + x];
					this.set_formula(current_cell, value);
				}
			}
			
			this.drawSheet();
		}

		this.findFirstTypeCell = function(startCell, row_or_column, direction, type){

			var currentCell = startCell;

			// check for row (row_or_column = 0), check for column (row_or_column = 1)
			while(true){
				currentCell[row_or_column] += direction; // decrements cell in case of direction -1

				if(type == 'nonempty'){

					if(this.get(currentCell) != undefined && this.get(currentCell) != ''){
						break;
					}
					else if(this.get(currentCell) == undefined){
						// undo last step to get to existent cell
						currentCell[row_or_column] -= direction;
						break;
					}

				}else if(type == 'empty'){

					if(this.get(currentCell) === undefined){
						
						// undo last step to get to existent cell
						currentCell[row_or_column] -= direction;
						break;
					}
					if(this.get(currentCell) == ''){

						// undo last step to get to existent cell
						currentCell[row_or_column] -= direction;
						break;
					}

				}else{
					break;
				}
			}

			return currentCell;
		}

		this.translateSelection = function(dx, dy, shift, ctrl){

			// set it equal to copy
			var cell = this.selectedCells[0].slice();

			
			if(shift){
				// create copy
				cell = this.selectedCells[1].slice();
			}

			if(ctrl){
				// transform dx and dy based on direction and first empty cell in this direction
				var row_or_column = 0;
				var direction = dy;

				if(dy == 0){
					row_or_column = 1;
					direction = dx;
				}

				// for empty cells go to first non-empty
				var currentNextCell = cell;


				var currentCellValue = this.get(currentNextCell);

				// if the current cell is empty not empty, check whether next cell is empty
				if(currentCellValue != ''){
					currentNextCell[0] += dy;
					currentNextCell[1] += dx; // move cell to next intended position

					var currentNextCellValue = this.get(currentNextCell);
					// protect against undefined location
					if(currentNextCellValue == undefined){
						currentNextCellValue = this.get(cell);
					}
				}else{
					var currentNextCellValue = currentCellValue;
				}
				

				if(currentNextCellValue == '' || currentNextCellValue == undefined){

					// console.log("Check for non empty cell");
					
					cell = this.findFirstTypeCell(cell, row_or_column, direction, 'nonempty');

				}
				// for non-empty cells go to first empty
				else{

					// console.log("Check for empty cell");
					
					cell = this.findFirstTypeCell(cell, row_or_column, direction, 'empty');

				}

			}else{
				cell = this.translateCell(cell, dx, dy);
			}

			// set back to global
			if(!shift){
				this.selectCell(cell);
			}

			// set second cell equal to first cell
			this.selectedCells[1] = cell;


			///// BLOCK: overflow key navigation and view-port correction

			// after re-position compare selectedCell (1) with visible cells
			var orderedCells = this.getSelectedCellsInOrder();

			var sheetViewWidth = this.sheetDom.clientWidth;
			var sheetViewHeight = this.sheetDom.clientHeight;

			if(orderedCells[0][0] < this.drawRowStart){

				// set vertical scroll to orderedCells[0][1] position
				var newScrollOffsetY = (this.sheetSizer.clientHeight - sheetViewHeight) * (orderedCells[0][0] / this.finalRow);
				this.sheetDom.scrollTop = newScrollOffsetY;

			}
			if(orderedCells[0][1] < this.drawColumnStart){

				// set horizontal scroll to orderedCells[0][1] position
				var newScrollOffsetX = (this.sheetSizer.clientWidth - sheetViewWidth) * (orderedCells[0][1] / this.finalColumn);
				this.sheetDom.scrollLeft = newScrollOffsetX;

			}

			// consider overflow on bottom end, compute boundarycell based on height/width data incremented from current drawRowstart
			var viewEndRow = this.drawRowStart;
			var measuredHeight = 0;

			// endless loop until maximum last row
			while(viewEndRow < this.numRows){

				measuredHeight += this.rowHeights[viewEndRow];
				
				// increment to next row
				if (measuredHeight >= (sheetViewHeight - this.sidebarSize)){

					// exclude finalRow since not fully in view
					viewEndRow--;
					break;
				}else{
					viewEndRow++;
				}
			}

			var viewEndColumn = this.drawColumnStart;
			var measureWidth = 0;

			
			// endless loop until maximum last row
			while(viewEndColumn < this.numColumns){

				measureWidth += this.columnWidths[viewEndColumn];

				// increment to next row
				if (measureWidth >= (sheetViewWidth - this.sidebarSize)){
					// exclude finalColumn since not fully in view
					viewEndColumn--;
					break;
				}else{
					viewEndColumn++;
				}
			}

			if(orderedCells[0][0] > viewEndRow){

				// compute the firstcell that needs to be selected in order to have the whole of the targetcell (orderedCells[0][0]) in view

				// compute downwards
				var minimumFirstRow = orderedCells[0][0];
				var measuredHeight = 0;
				
				// endless loop until maximum last row
				while(minimumFirstRow >= 0){

					measuredHeight += this.rowHeights[minimumFirstRow];
					
					// increment to next row
					if (measuredHeight >= (sheetViewHeight - this.sidebarSize)){
						// exclude final row since not fully in view
						minimumFirstRow++;
						break;
					}else{
						minimumFirstRow--;
					}
				}
				
				// set vertical scroll to orderedCells[0][1] position
				var newScrollOffsetY = (this.sheetSizer.clientHeight - sheetViewHeight) * (minimumFirstRow / this.finalRow);
				this.sheetDom.scrollTop = newScrollOffsetY;

			}

			if(orderedCells[0][1] > viewEndColumn){

				// compute downwards
				var minimumFirstColumn = orderedCells[0][1];
				var measureWidth = 0;
				
				// endless loop until maximum last row
				while(minimumFirstColumn >= 0){

					measureWidth += this.columnWidths[minimumFirstColumn];
					
					// increment to next row
					if (measureWidth >= (sheetViewWidth - this.sidebarSize)){
						// exclude final row since not fully in view
						minimumFirstColumn++;
						break;
					}else{
						minimumFirstColumn--;
					}
				}

				// set horizontal scroll to orderedCells[0][1] position
				var newScrollOffsetX = (this.sheetSizer.clientWidth - sheetViewWidth) * (minimumFirstColumn / this.finalColumn);
				this.sheetDom.scrollLeft = newScrollOffsetX;

			}

			this.scrollOffsetX = this.sheetDom.scrollLeft;
			this.scrollOffsetY = this.sheetDom.scrollTop;

			// redraw
			this.drawSheet();
		}

		this.translateCell = function(cell, dx, dy){
			
			// row
			cell[0] += dy;

			// column
			cell[1] += dx;
			

			if(cell[0] < 0){
				cell[0] = 0;
			}
			if(cell[1] < 0){
				cell[1] = 0;
			}
			return cell;
		}

		this.cellLocationToPosition = function(cellPosition){
			
			// return the X, Y coordinates of the cell or undefined if the cell is not being rendered
			
			// check whether the cells are within the view bound
			// if(cellPosition[0] < this.drawRowStart || cellPosition[1] < this.drawColumnStart){
			// 	return undefined;
			// }else{

				
			// }

			// TODO: for now don't check bounds

			// calculate the y axis (the row)
			var y = 0;
			var currentRowHeight = 0;
			for(var i = 0; i < this.numRows; i++){

				if(i == cellPosition[0]){
					y = currentRowHeight - this.sheetOffsetY;
					break;
				}
				// check for out of bounds
				if(currentRowHeight - this.sheetOffsetY > this.sheetDom.clientHeight){
					return undefined;
				}

				currentRowHeight += this.rowHeights[i];
			}

			// calculate x axis (the column)
			var x = 0;
			var currentColumnWidth = 0;

			for(var i = 0; i < this.numColumns; i++){

				if(i == cellPosition[1]){
					x = currentColumnWidth - this.sheetOffsetX;
					break;
				}
				// check for out of bounds
				if(currentColumnWidth - this.sheetOffsetX > this.sheetDom.clientWidth){
					return undefined;
				}

				currentColumnWidth += this.columnWidths[i];
			}

			return [x, y];
		}

		this.positionToCellLocation = function(x, y){

			var rowX = x + this.sheetOffsetX - this.sidebarSize;
			var columnIndex = 0;
			var currentColumnWidth = 0;

			for(var i = 0; i < this.numColumns; i++){

				currentColumnWidth += this.columnWidths[i];
				if(currentColumnWidth >= rowX){
					columnIndex = i;
					break;
				}
			}

			var rowY = y + this.sheetOffsetY - this.sidebarSize;
			var rowIndex = 0;
			var currentRowHeight = 0;

			for(var i = 0; i < this.numRows; i++){
				currentRowHeight += this.rowHeights[i];
				if(currentRowHeight >= rowY){
					rowIndex = i;
					break;					
				}
			}

			return [rowIndex, columnIndex];
		}

		this.positionToCellDivider = function(x, y, type){
			
			var rowIndex = 0;
			var columnIndex = 0;
			
			// optimize efficiency due to never being able to resize both column and row
			if(type == 'column'){
				var rowX = x + this.sheetOffsetX - this.sidebarSize;
				var currentColumnWidth = 0;
	
				for(var i = 0; i < this.numColumns; i++){
	
					currentColumnWidth += this.columnWidths[i];
					if(currentColumnWidth >= rowX){
						columnIndex = i;
	
						var dist1 = Math.abs(rowX - currentColumnWidth);
						var dist2 = Math.abs(rowX - (currentColumnWidth - this.columnWidths[i]));
	
						// if currentColumndWidth -= this.columnsWidths[i] is closer, choose that column
						if(dist2 < dist1){
							columnIndex = i - 1;
						}
						
						break;
					}
				}
			}else{
				var rowY = y + this.sheetOffsetY - this.sidebarSize;
				var currentRowHeight = 0;
	
				for(var i = 0; i < this.numRows; i++){
					currentRowHeight += this.rowHeights[i];
					if(currentRowHeight >= rowY){
						rowIndex = i;
	
						var dist1 = Math.abs(rowY - currentRowHeight);
						var dist2 = Math.abs(rowY - (currentRowHeight - this.rowHeights[i]));
	
						// if currentRowHeight -= this.rowHeights[i] is closer, choose that row
						if(dist2 < dist1){
							rowIndex = i - 1;
						}
	
						break;					
					}
				}
			}

			return [rowIndex, columnIndex];
		}

		this.initRowCols = function(){

			// config
			this.cellHeight = 20;
			this.cellWidth = 100;

			for(var y = 0; y < this.numRows; y++){
				this.rowHeights.push(this.cellHeight);
				// this.rowHeights.push(Math.round(Math.random()*50) + 20);
			}

			for(var x = 0; x < this.numColumns; x++){
				this.columnWidths.push(this.cellWidth);
				// this.columnWidths.push(Math.round(Math.random()*200) + 50);
			}

			// add for testing
			// this.columnWidths[3] = 200;
			// this.rowHeights[2] = 50;
			// this.rowHeights[3] = 50;

			this.computeRowHeight();
			this.computeColumnWidth();

			this.computeScrollBounds();
			
		}

		this.computeScrollBounds = function(){

			var width = this.sheetDom.clientWidth;
			var height = this.sheetDom.clientHeight;

			var totalHeight = 0;
			var totalWidth = 0;
			var finalColumn = 0;
			var finalRow = 0;

			for(var y = this.numRows-1; y >= 0; y--){

				totalHeight += this.rowHeights[y];
				if(totalHeight < height-this.sidebarSize){
					finalRow = y; // choose starting cell that guarantees that it will be in view
				}else{
					break;
				}
				
			}

			// interpolate linearly between 0 and finalRow
			for(var x = this.numColumns-1; x >= 0; x--){

				totalWidth += this.columnWidths[x];
				if(totalWidth < width-this.sidebarSize){
					finalColumn = x + 1;
				}else{
					break;
				}
			}
			this.finalRow = finalRow;
			this.finalColumn = finalColumn;

		}
		
		this.codeOpen = true;
		
		this.resizeSheet = function(){
			
			this.computeScrollBounds();
			this.sizeSizer();
			this.drawSheet();
			
		}
		
		this.toggleCode = function(){
			if (this.codeOpen){
				
				// close editor
				$(this.editor.dom).css({width: 0})
				$(this.sheetDom).css({width: '100%'});
				
			}else{
				
				// open editor
				$(this.editor.dom).css({width: ''})
				$(this.sheetDom).css({width: ''});
				
			}
			
			// resize spreadsheet
			this.resizeSheet();
			
			this.codeOpen = !this.codeOpen;
		}
		
		this.openFile = function(){
			
			var input = $(this.dom).find('menu-item.load-csv input');
			input.click();
		}
		
		this.uploadFile = function(){
				
			var input = $(this.dom).find('menu-item.load-csv input');
			
			var reader = new FileReader();
			
			reader.onload = function(e){
				
				var data = e.target.result;
				
				// console.log(data);
				
				// send data through WS
				_this.wsManager.send(JSON.stringify({arguments: ["CSV", data]}));
			}
			
			reader.readAsText(input[0].files[0]);
			
		}

		this.menuInit = function(){

			var menu = $(this.dom).find('div-menu');

			menu.find('menu-item.about').click(function(){
				alert("This is a web-based Spreadsheet program built by R. Lamers");
			});

			menu.find('menu-item.plot-scatter').click(function(){
				_this.plot('scatter');
			});
			menu.find('menu-item.plot-line').click(function(){
				_this.plot('line');
			});

			menu.find('menu-item.plot-histogram').click(function(){
				_this.plot('histogram');
			});
			
			menu.find('menu-item.code').click(function(){
				_this.toggleCode();
			});
			
			
			// set up file change handler for loadCSV
			var input = $(this.dom).find('menu-item.load-csv input');
			
			input[0].addEventListener('change', function(e){
				_this.uploadFile();
				console.log(e);
			})
			
			menu.find('menu-item.load-csv').click(function(e){
				if(!$(e.target).hasClass('file-input')){
					_this.openFile();
				}
			});
			
			// bind for later access
			this.menu = menu;

			// bind plot activate functions
			$(document).on('click', 'menu-item.plot-item', function(){

				var plot_id = $(this).attr('data-plot-id');

				var plot = $("#"+plot_id).parents('.plot');

				if(plot.is(':visible')){
					plot.hide();
					$(this).removeClass('active');
				}else{
					plot.show();
					$(this).addClass('active');
				}
				
			});
		}

		this.parseFloatForced = function(x){
			var num = parseFloat(x);
			if(isNaN(num)){
				num = 0;
			}
			return num;
		}

		this.plot_count = 0;

		// init at 9
		this.plot_z_index = 9;

		this.plot_draggable = function(elem){
			
			var $elem = $(elem);
			var mouse_down = false;
			var resize = false;
			var curPosition;

			// initialize on current css value
			var position = $elem.position()
			var transform = [position.left, position.top];

			var plot_id = $elem.find('.plotly-holder').attr('id');

			$elem.find('.close').click(function(){
				// remove plot
				delete _this.plots[plot_id];
				
				// remove element
				$($elem).remove();

				// remove from menu
				_this.menu.find('menu-item[data-plot-id="'+plot_id+'"]').remove();
				
				if(_this.menu.find('.plot-list menu-item').length == 1){
					_this.menu.find('.no-plots').show();
				}
			
			})

			$elem.find('.minimize').click(function(){

				// animation
				$elem.addClass('animate');

				
				var position = $elem.position()
				var oldTransform = [position.left, position.top];

				// move transform to upper left
				$elem.css({ transform: "translate("+ (-oldTransform[0]) +"px,"+ (-oldTransform[1]) +"px)", opacity: 0});
				
				setTimeout(function(){

					// remove active from menu
					_this.menu.find('menu-item[data-plot-id="'+plot_id+'"]').removeClass('active');
					$elem.hide();
					$elem.removeClass('animate');

					// restore pre-animation variables
					$elem.css({opacity: 1, transform: ''});
				
				},300);
				
			})
			
			$elem.find('.save-svg').click(function(){
				
				Plotly.toImage(plot_id,{format:'svg', width:$elem.width(), height:$elem.height()}).then(function(data){
					
					dataURLtoBytes(data).then(function(data){
						download(data, plot_id + ".svg", "svg");
					})
				});
			
			});
			

			elem.addEventListener('mousedown', function(e){
				mouse_down = true;
				curPosition = [e.clientX, e.clientY];

				// increment z-index 
				_this.plot_z_index++;

				$elem.css({zIndex: _this.plot_z_index});

				if($(e.target).hasClass('resizer')){
					resize = true;
				}
			});

			document.addEventListener('mousemove', function(e){
				if(mouse_down){
					var diff = [e.clientX - curPosition[0], e.clientY - curPosition[1]];

					if(!resize){

						transform[0] += diff[0];
						transform[1] += diff[1];

						// move
						$elem.css({left: transform[0] + "px", top: transform[1] + "px"})
						
					}else{
						// resize
						var plotly_holder = $elem.find('.plotly-holder');
						var width = plotly_holder.width();
						var height = plotly_holder.height();

						width =width+diff[0];
						height = height+diff[1];

						plotly_holder.css({width: width+"px", height: height+"px" });

						var update = {
							width: width,  // or any new width
							height: height  // " "
						};

						Plotly.relayout(plot_id, update);
					}

					

					curPosition = [e.clientX, e.clientY];
				}
				
			});
			document.addEventListener('mouseup',function(){
				mouse_down = false;
				resize = false;
			})
		}

		this.getSelectedCellsInOrder = function() {

			var selectedCellsCopy = [this.selectedCells[0],this.selectedCells[1]];
			if(selectedCellsCopy[0][0] >= selectedCellsCopy[1][0] && selectedCellsCopy[0][1] >= selectedCellsCopy[1][1]){
				// swap
				var tmp = selectedCellsCopy[0];
				selectedCellsCopy[0] = selectedCellsCopy[1];
				selectedCellsCopy[1] = tmp;
			}

			return selectedCellsCopy;
		}
		
		this.get_range_float = function(range){
			return this.get_range(range[0],range[1]).map(this.parseFloatForced);
		}
		
		this.update_plot = function(plot){
			var x_range = plot.data[0];
			var y_range = plot.data[1];
			
			var data_update;

			if (x_range.length > 0){
				data_update = {
					x: this.get_range_float(x_range),
				};
				Plotly.restyle(plot.plot_id, 'x', [data_update.x]);
			}
			
			if(y_range.length > 0){
				data_update = {
					y: this.get_range_float(y_range)
				};
				Plotly.restyle(plot.plot_id, 'y', [data_update.y]);
			}

			// recompute 
			if(plot.type == 'histogram'){
				// TODO: re-compute the histogram bins
			}
			
		}

		this.plot = function(type){

			
			var x_range = [];
			var y_range = [];

			var selectedCellsOrdered = this.getSelectedCellsInOrder();

			// get current data range
			if(type == 'scatter'){

				// check width of selection
				if(selectedCellsOrdered[0][1] == selectedCellsOrdered[1][1]){
					alert("Scatter plot requires two columns");
					return;
				}
				
				x_range = [[selectedCellsOrdered[0][0],selectedCellsOrdered[0][1]],[selectedCellsOrdered[1][0],selectedCellsOrdered[1][1]-1]];
				y_range = [[selectedCellsOrdered[0][0],selectedCellsOrdered[0][1]+1],[selectedCellsOrdered[1][0],selectedCellsOrdered[1][1]]];

				var trace1 = {
					x: this.get_range_float(x_range),
					y: this.get_range_float(y_range),
					mode: 'markers',
					type: 'scatter'
				};

			}
			
			if(type == 'histogram'){
				
				x_range = [[selectedCellsOrdered[0][0],selectedCellsOrdered[0][1]],[selectedCellsOrdered[1][0],selectedCellsOrdered[1][1]]];
				
				var trace1 = {
					x: this.get_range_float(x_range),
					type: 'histogram',
					autobinx: true
				};
				// var minArray = getMinOfArray(trace1.x);
				// var maxArray = getMaxOfArray(trace1.x);
				// var bincount = (trace1.x.length/5);
				// trace1.xbins = {start:minArray, end: maxArray, size: maxArray / bincount};

			}

			if(type == 'line'){

				// detect if two or single column
				if(selectedCellsOrdered[0][1] != selectedCellsOrdered[1][1]){
					x_range = [[selectedCellsOrdered[0][0],selectedCellsOrdered[0][1]],[selectedCellsOrdered[1][0],selectedCellsOrdered[1][1]-1]];
					y_range = [[selectedCellsOrdered[0][0],selectedCellsOrdered[0][1]+1],[selectedCellsOrdered[1][0],selectedCellsOrdered[1][1]]];
					
					var trace1 = {
						x: this.get_range_float(x_range),
						y: this.get_range_float(y_range),
						mode: 'lines',
						type: 'scatter'
					};
				}else{

					y_range = [[selectedCellsOrdered[0][0],selectedCellsOrdered[0][1]],[selectedCellsOrdered[1][0],selectedCellsOrdered[1][1]]];
					
					var trace1 = {
						y: this.get_range_float(y_range),
						mode: 'lines',
						type: 'scatter'
					};

				}
				
			}
			
			// increment plot count after validation steps

			this.plot_count++;

			var plot_id = "plot-" + this.plot_count;
			var plot_div = $('<div class="plot scatter"><div class="resizer"></div><div class="plot-header"><div class="close"><img src="image/cross.svg" /></div><div class="minimize"><img src="image/dash.svg" /></div><div class="save-svg"><img src="image/floppy.svg" /></div></div><div class="plotly-holder" id="'+plot_id+'" ></div></div>');

			// position in the middle
			var plotWidth = 520;
			var plotHeight = plotWidth / (16/9);

			var offsetX = (this.sheetDom.clientWidth - plotWidth)/2;
			var offsetY = this.sheetDom.clientHeight * 0.1;

			plot_div.find("#"+plot_id).css({width: plotWidth, height: plotHeight});
			plot_div.css({left: offsetX + "px", top: offsetY + "px"})

			$('.main-body').prepend(plot_div);
			
			var data = [trace1];
			
			var layout = {
				title: type.capitalize() + " plot",
				showlegend: false,
				margin: {
					l: 40,
					r: 40,
					b: 40,
					t: 100,
					pad: 4},
				};

			Plotly.setPlotConfig({
				modeBarButtonsToRemove: ['sendDataToCloud']
			});
			
				
			Plotly.newPlot(plot_id, data,layout,{scrollZoom: true});

			this.addPlotToMenu(plot_id);
			this.plot_draggable(plot_div[0]);
			
			// add plot
			this.plots[plot_id] = {plot_id, type: type, data: [x_range, y_range]}


		}

		this.addPlotToMenu = function(plot_id){
			var menuList = this.menu.find('menu-list.plot-list');
			menuList.find('.no-plots').hide();
			menuList.append("<menu-item class='plot-item active' data-plot-id='"+plot_id+"'>Plot "+ this.plot_count +"</menu-item>")
		}

		this.computeColumnWidth = function(){
			var sum = this.columnWidths.reduce(function(total, num){ return total + num; }, 0);
			this.computedColumnWidth = sum;
			return sum;
		}
		this.computedColumnWidth = this.computeColumnWidth();
		
		this.computeRowHeight = function(){
			var sum = this.rowHeights.reduce(function(total, num){ return total + num; }, 0);
			this.computedRowHeight = sum;
			return sum;
		}
		this.computedRowHeight = this.computeRowHeight();

		this.drawSheet = function(){
			var width = this.sheetDom.clientWidth;
			var height = this.sheetDom.clientHeight;

			this.ctx.strokeStyle = '#bbbbbb';
			this.ctx.lineWidth = 1;
			this.ctx.clearRect(0, 0, width, height);

			
			// incorporate offset in drawing

			// draw row lines
			var drawRowStart = undefined;
			var drawColumnStart = undefined;
			var measureHeight = 0;
			var measureWidth = 0;

			var firstCellHeightOffset = 0;
			var firstCellWidthOffset = 0;

			// figure out where to start drawing based on scrollOffsetY
			// TODO: replace this by percentage method that will scale well with large datasets
			var columnPercentage = this.scrollOffsetX / (this.sheetSizer.clientWidth-width);
			var rowPercentage = this.scrollOffsetY / (this.sheetSizer.clientHeight-height);

			// percentage method
			var drawRowStart = Math.round(rowPercentage * this.finalRow);
			var drawColumnStart = Math.round(columnPercentage * this.finalColumn);

			this.drawRowStart = drawRowStart;
			this.drawColumnStart = drawColumnStart;

			for(var x = 0; x < this.numColumns; x++){
				if(x == this.drawColumnStart){
					break;
				}
				measureWidth += this.columnWidths[x];
				
			}

			for(var x = 0; x < this.numRows; x++){
				if(x == this.drawRowStart){
					break;
				}
				measureHeight += this.rowHeights[x];
				
			}
			

			// empty cell catch
			if(drawRowStart === undefined){
				drawRowStart = this.numRows-1;
			}
			if(drawColumnStart === undefined){
				drawColumnStart = this.numColumns-1;
			}


			this.sheetOffsetY = measureHeight;
			this.sheetOffsetX = measureWidth;

			var i = drawRowStart;
			var drawHeight = 0;
			var currentY = 0;

			this.ctx.beginPath();

			// render bars below navigation
			this.ctx.fillStyle = "#eeeeee";
			this.ctx.fillRect(0, 0, this.sidebarSize, height);
			this.ctx.fillRect(0, 0, width, this.sidebarSize);
			this.ctx.fillStyle = "#000000";
			

			// render horizontal lines
			// render grid

			// draw top line
			this.ctx.a_moveTo(0, 0);
			this.ctx.a_lineTo(width, 0);

			while(true){

				if(currentY > height || i > this.numRows){
					break;
				}

				// draw row holder lines
				this.ctx.a_moveTo(0, currentY + firstCellHeightOffset + this.sidebarSize);
				this.ctx.a_lineTo(width, currentY + firstCellHeightOffset + this.sidebarSize);

				currentY += this.rowHeights[i];

				i++;
			}

			// render vertical lines
			var d = drawColumnStart;
			var drawWidth = 0;
			var currentX = 0;

			while(true){
				
				if(currentX > width || d > this.numColumns){
					break;
				}

				this.ctx.a_moveTo(currentX + firstCellWidthOffset + this.sidebarSize, 0);
				this.ctx.a_lineTo(currentX + firstCellWidthOffset + this.sidebarSize, height);

				currentX += this.columnWidths[d];

				d++;
			}

			this.ctx.closePath();

			this.ctx.stroke();

			// this render highlight
			this.renderHighlights();

			// render cell data
			this.renderCells(drawRowStart, drawColumnStart, i, d, firstCellHeightOffset, firstCellWidthOffset);

			
			// also re-render the input_formula field
			this.updateInputFormula();
			
		}
		
		this.updateInputFormula = function(){
			this.formula_input.val(this.get_formula(this.selectedCells[0]));
		}

		this.cellRangeDistance = function(cell1, cell2){
			var xCellDistance = cell2[1] - cell1[1];
			var yCellDistance = cell2[0] - cell1[0];

			return [xCellDistance, yCellDistance];
		}

		this.cellRangeSize = function(cell1, cell2){

			var xCellDistance = (cell2[1] - cell1[1]) + 1;
			var yCellDistance = (cell2[0] - cell1[0]) + 1;

			return [xCellDistance, yCellDistance];
		}


		this.renderHighlights = function(){
			
			if(this.selectedCells){
				
				// selectedCellStart is filled
				this.ctx.fillStyle = "rgba(50, 110, 255, 0.20)";

				var cellsForSelected = this.getSelectedCellsInOrder();

				var cell_position = this.cellLocationToPosition(cellsForSelected[0]);

				// check if selected cell is in the viewport
				if(cell_position){
					// selectedCell index = first cell, first index is row, second index is column
					var highlightWidth = 0;
					var highlightHeight = 0;

					var cellRangeSize = this.cellRangeDistance(cellsForSelected[0], cellsForSelected[1]);

					var xCellDistance = cellRangeSize[0];
					var yCellDistance = cellRangeSize[1];

					var shiftY = 0;
					var shiftX = 0;

					if(xCellDistance < 0){
						for(var x = 0; x >= xCellDistance;x--){
							highlightWidth -= this.columnWidths[cellsForSelected[0][1] + x];						
							
							if(x == 0){
								shiftX = Math.abs(highlightWidth);
							}
						}
					}else{
						for(var x = 0; x <= xCellDistance; x ++){
							highlightWidth += this.columnWidths[cellsForSelected[0][1] + x];
						}
					}

					if(yCellDistance < 0){
						for(var y = 0; y >= yCellDistance; y--){
							highlightHeight -= this.rowHeights[cellsForSelected[0][0] + y];

							if(y == 0){
								shiftY = Math.abs(highlightHeight);
							}
						}
					}else{
						for(var y = 0; y <= yCellDistance; y ++){
							highlightHeight += this.rowHeights[cellsForSelected[0][0] + y];
						}
					}
					
					
					var drawX = cell_position[0] + shiftX + this.sidebarSize;
					var drawY = cell_position[1] + shiftY + this.sidebarSize;
					var drawWidth = highlightWidth;
					var drawHeight = highlightHeight;

					// clip x and y start to this.sidebarSize
					if (drawX < this.sidebarSize){
						drawWidth = drawWidth - (this.sidebarSize - drawX);
						if(drawWidth < 0){
							drawWidth = 0;
						}
						drawX = this.sidebarSize;
					}
					if (drawY < this.sidebarSize){
						drawHeight = drawHeight - (this.sidebarSize - drawY);
						if(drawHeight < 0){
							drawHeight = 0;
						}
						drawY = this.sidebarSize;
					}
					
					this.ctx.fillRect(
						drawX,
						drawY, 
						drawWidth, 
						drawHeight);
				}

			}

		}
		

		this.renderCells = function(startRow, startColumn, endRow, endColumn, firstCellHeightOffset, firstCellWidthOffset){

			this.ctx.font = this.fontStyle;
			this.ctx.fillStyle = "black";
			this.ctx.textAlign = 'left';
			this.ctx.textBaseline = 'top';

			var currentX = 0;
			var currentY = 0;
			var startX = 0;

			// if(startRow >= 1){
			// 	startRow--;
			// 	currentY -= this.rowHeights[startRow];
			// }
			// if(startColumn >= 1){
			// 	startColumn--;
			// 	startX -= this.columnWidths[startColumn];
			// 	currentX = startX;
			// }
			
			// render one more 
			for(var i = startRow; i < endRow; i++){

				for(var d = startColumn; d < endColumn; d++){

					// compensate for borders (1px)
					var centeringOffset = ((this.rowHeights[i] + 2 - this.fontHeight)/2) + 1;

					// get data
					var cell_data = this.get([i, d]);

					var cellMaxWidth = this.columnWidths[d] - this.textPadding - 2; // minus borders

					if(cell_data !== undefined){

						this.ctx.textAlign = 'left';

						var fitted_cell_data = this.fittingStringFast(cell_data, cellMaxWidth);
						this.ctx.fillText(fitted_cell_data, currentX + firstCellWidthOffset + this.textPadding + this.sidebarSize, currentY + firstCellHeightOffset + centeringOffset + this.sidebarSize);
					}


					// for the first row, render the column headers
					if (i == startRow) {
						this.ctx.textAlign = 'center';

						var centerOffset = this.columnWidths[d]/2;
						var centeringOffset = ((this.sidebarSize + 2 - this.fontHeight)/2) + 1;
					
						this.ctx.fillText(this.indexToLetters(d+1), currentX + firstCellWidthOffset + this.sidebarSize + centerOffset, centeringOffset);
					}

					currentX += this.columnWidths[d];
					
					
				}

				this.ctx.textAlign = 'center';
				var centerOffset = this.sidebarSize/2;
				var centeringOffset = ((this.rowHeights[i] + 2 - this.fontHeight)/2) + 1;

				this.ctx.fillText(i+1, firstCellWidthOffset + centerOffset, currentY + firstCellHeightOffset + this.sidebarSize + centeringOffset);

				currentY += this.rowHeights[i];

				// reset currentX for next iteration
				currentX = startX;
			}

		}

		this.computeWLetterSize = function(){
			var width = this.ctx.measureText("W").width;
			return width;
		}

		this.cachedWLetterSize = this.computeWLetterSize();

		this.fittingStringFast = function(str, maxWidth){
			if(str.length * this.cachedWLetterSize < maxWidth){
				return str;
			}else if(str.length > (maxWidth/this.cachedWLetterSize) * 2 ){
				return str.substring(0, maxWidth/this.cachedWLetterSize) + "...";
			}
			else{
				return fittingString(this.ctx, str, maxWidth);
			}
		}
	}

	

	function fittingString(c, str, maxWidth) {
		var width = c.measureText(str).width;
		if(width < maxWidth){
			return str;
		}else{
			var ellipsis = '…';
			var ellipsisWidth = c.measureText(ellipsis).width;
			var len = str.length;
			while (width>=maxWidth-ellipsisWidth && len-->0) {
				str = str.substring(0, len);
				width = c.measureText(str).width;
			}
			return str+ellipsis;
		}
	}


	var determineFontHeight = function(fontStyle) {
		var body = document.getElementsByTagName("body")[0];
		var dummy = document.createElement("div");
		var dummyText = document.createTextNode("M");
		dummy.appendChild(dummyText);
		dummy.setAttribute("style", fontStyle);
		body.appendChild(dummy);
		var result = dummy.offsetHeight;
		body.removeChild(dummy);
		return result;
	};

	var app = new App();
	app.init();
	window.app = app;

})();