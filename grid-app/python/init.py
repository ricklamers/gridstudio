import json
import traceback
import re
import matplotlib
import sys
matplotlib.use('Agg')

import base64
import os
import numbers
import matplotlib.pyplot as plt


if os.path.isdir("/home/user"):
    os.chdir("/home/user")

sheet_data = {}

real_print = print

def print(text):
    real_print("#INTERPRETER#" + str(text) + "#ENDPARSE#", end='', flush=True)

def parseCall(*arg):
    result = ""
    try:
        eval_result = eval(arg[0] + "(\""+'","'.join(arg[1:])+"\")")

        if isinstance(eval_result, numbers.Number) and not isinstance(eval_result, bool):
            result = str(eval_result)
        else:
            result = "\"" + str(eval_result) + "\""
        
    except (RuntimeError, TypeError, NameError):
        result = "\"" + "Unexpected error:" + str(sys.exc_info()) + "\""
        
    # real_print()
    # Method does not exist.  What now?
    # result = result + "\"Unknown function was called with: " + str(arg) + "\""
        # result = "\"result + str(" + arg[0] + "(\""+'\",\"'.join(arg[1:])+"\"))\""

    real_print("#PYTHONFUNCTION#"+result+"#ENDPARSE#", flush=True, end='')

def cell(cell, value = None):
    if value is not None:
        # set value
        sheet(cell, value)
    else:
        # just return value
        cell_range = ':'.join([cell, cell])
        return sheet(cell_range)

def plot():
    plt.savefig("tmp.png")
    with open("tmp.png", "rb") as image_file:
        encoded_string = base64.b64encode(image_file.read())

    image_string = str(encoded_string)
    data = {'arguments': ["IMAGE", image_string[2:len(image_string)-1]]}
    data = ''.join(['#IMAGE#', json.dumps(data),'#ENDPARSE#'])

    real_print(data, flush=True, end='')

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


def has_number(s):
    return any(i.isdigit() for i in s)

def convert_to_json_string(element):

    if isinstance(element, str):
        # string meant as string, escape
        return "\"" + element + "\""
    else:
        return format(element, '.12f')

def sheet(cell_range, data = None):

    # input data into sheet
    if data is not None:

        # convert numpy to array
        data_type_string = str(type(data))
        if data_type_string == "<class 'numpy.ndarray'>":
            data = data.tolist()

        # always convert cell to range
        if ":" not in cell_range:
            if not has_number(cell_range):
                if type(data) is list:
                    cell_range = cell_range + "1:" + cell_range + str(len(data))
                else :
                    cell_range = cell_range + "1:" + cell_range + "1"
            else:
                cell_range = cell_range + ":" + cell_range
        
        
        
            
        if type(data) is list:
            
            newList = list(map(convert_to_json_string, data))

            arguments =  ['RANGE', 'SETLIST', cell_range]

            # append list
            arguments = arguments + newList

            json_object = {'arguments':arguments}
            json_string = ''.join(['#PARSE#', json.dumps(json_object),'#ENDPARSE#'])
            real_print(json_string, flush=True, end='')

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
            data = ''.join(['#PARSE#', json.dumps(data),'#ENDPARSE#'])
            real_print(data, flush=True, end='')
    
    # get data from sheet
    else:
        #convert non-range to range for get operation
        if ":" not in cell_range:
            cell_range = ':'.join([cell_range, cell_range])

        # in blocking fashion get latest data of range from Go
        real_print("#DATA#" + cell_range + '#ENDPARSE#', end='', flush=True)
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
                real_print("#COMMANDCOMPLETE##ENDPARSE#", end='', flush = True)
            except:
                traceback.print_exc()
            command_buffer = ""
        else:
            command_buffer += code_input + "\n"

# testing
#sheet("A1:A2", [1,2])
getAndExecuteInput()
