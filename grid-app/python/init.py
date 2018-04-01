import json
import traceback
import re

sheet_data = {}

def cell(cell, value = None):
    if value is not None:
        # set value
        sheet(cell, value)
    else:
        # just return value
        cell_range = ':'.join([cell, cell])
        sheet(cell_range)
        return sheet_data[cell]

def getReferenceRowIndex(reference):
    return int(re.findall(r'\d+', reference)[0])

def getReferenceColumnIndex(reference):
    return letterToIndex(''.join(re.findall(r'[a-zA-Z]', reference)))

def letterToIndex(letters):
    columns = len(letters) - 1
    total = 0
    base = 26

    for x in letters:
        number = ord(x)-64
        total += number * int(base**columns)
        columns -= 1
    return total

def indexToLetters(index):

    base = 26

    # start at the base that is bigger and work your way down
    leftOver = index

    columns = []

    while leftOver > 0:
        remainder = leftOver % base
        
        if remainder == 0:
            remainder = base

        columns.insert(0, int(remainder))
        leftOver = (leftOver - remainder) / base

    buff = ""

    for x in columns:
        buff += chr(x + 64)

    return buff

def cell_range_to_indexes(cell_range):
    references = []

    cells = cell_range.split(":")

    cell1Row = getReferenceRowIndex(cells[0])
    cell2Row = getReferenceRowIndex(cells[1])

    cell1Column = getReferenceColumnIndex(cells[0])
    cell2Column = getReferenceColumnIndex(cells[1])

    for x in range(cell1Column, cell2Column+1):
        for y in range(cell1Row, cell2Row+1):
            references.append(indexToLetters(x) + str(y))

    return references

def sheet(cell_range, data = None):

    # input data into sheet
    if data is not None:
        
        # always convert cell to range
        if ":" not in cell_range:
            cell_range = cell_range + ":" + cell_range
        
        
        # convert numpy to array
        data_type_string = str(type(data))
        if data_type_string == "<class 'numpy.ndarray'>":
            data = data.tolist()
            
        if type(data) is list:
            
            # if data is string without starting with =, add escape quotes
            for index, element in enumerate(data):

                if isinstance(element, str):
                    # string meant as string, escape
                    element = "\"" + element + "\""
                    data[index] = element
                else:
                    data[index] = str(element)

            arguments =  ['RANGE', 'SETLIST', cell_range]

            # append list
            arguments = arguments + data

            data = {'arguments':arguments}
            data = ''.join(['#PARSE#', json.dumps(data)])
            print(data, flush=True)

        else:

            # if data is string without starting with =, add escape quotes
            if isinstance(data, str) and data[0] == '=':
                # string meant as direct formula
                # do nothing
                data = data[1:]
            elif isinstance(data, str):
                # string meant as string, escape
                data = "\"" + data + "\""

            data = {'arguments': ['RANGE', 'SETSINGLE', cell_range, ''.join(["=",str(data)])]}
            data = ''.join(['#PARSE#', json.dumps(data)])
            print(data, flush=True)
    
    # get data from sheet
    else:
        #convert non-range to range for get operation
        if ":" in cell_range:
            cell_range = ':'.join([cell_range, cell_range])

        # in blocking fashion get latest data of range from Go
        print("#DATA#" + cell_range, flush=True)
        getAndExecuteInputOnce()
        # if everything works, the exec command has filled sheet_data with the appropriate data
        # return data range as arrays
        cell_refs = cell_range_to_indexes(cell_range)

        result = []
        for cell_ref in cell_refs:
            result.append(sheet_data[cell_ref])

        return result

def getAndExecuteInputOnce():

    command_buffer = ""

    while True:

        code_input = input("")
        
        # when empty line is found, execute code
        if code_input == "":
            try:
                exec(command_buffer, globals(), globals())
            except:
                traceback.print_exc()
            return
        else:
            command_buffer += code_input + "\n"

def getAndExecuteInput():

    command_buffer = ""

    while True:
        code_input = input("")
        # when empty line is found, execute code
        if code_input == "":
            try:
                exec(command_buffer, globals(), globals())
            except:
                traceback.print_exc()
            command_buffer = ""
        else:
            command_buffer += code_input + "\n"

# testing
#sheet("A1:A2", [1,2])
getAndExecuteInput()
