import warnings
warnings.filterwarnings("ignore", message="numpy.dtype size changed")
warnings.filterwarnings("ignore", message="numpy.ufunc size changed")

import json
import traceback
import re
import matplotlib
import sys
import pandas as pd
import numpy as np

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
    if not isinstance(text, str):
        text = str(text)

    real_print("#INTERPRETER#" + text + "#ENDPARSE#", end='', flush=True)

def parseCall(*arg):
    result = ""
    try:
        if len(arg) > 1:
            eval_result = eval(arg[0] + "(\""+'","'.join(arg[1:])+"\")")
        else:
            eval_result = eval(arg[0] + "()")

        if isinstance(eval_result, numbers.Number) and not isinstance(eval_result, bool):
            result = str(eval_result)
        else:
            result = "\"" + str(eval_result) + "\""
        
    except (RuntimeError, TypeError, NameError):
        result = "\"" + "Unexpected error:" + str(sys.exc_info()) + "\""
        
    real_print("#PYTHONFUNCTION#"+result+"#ENDPARSE#", flush=True, end='')

def cell(cell, value = None):
    if value is not None:
        # set value
        sheet(cell, value)
    else:
        # just return value
        cell_range = ':'.join([cell, cell])
        return sheet(cell_range)

def show():
    plt.savefig("tmp.svg")
    with open("tmp.svg", "rb") as image_file:
        encoded_string = base64.b64encode(image_file.read())

    image_string = str(encoded_string)
    data = {'arguments': ["IMAGE", image_string[2:len(image_string)-1]]}
    data = ''.join(['#IMAGE#', json.dumps(data),'#ENDPARSE#'])

    # remove to clean up
    os.remove("tmp.svg")

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
        columnReferences = []
        for y in range(cell1Row, cell2Row+1):
            columnReferences.append(indexToLetters(x) + str(y))
        references.append(columnReferences)
        
    return references


def has_number(s):
    return any(i.isdigit() for i in s)

def convert_to_json_string(element):

    if isinstance(element, str):
        # string meant as string, escape
        element = element.replace("\n","")

        # if data is string without starting with =, add escape quotes
        if len(element) > 0 and element[0] == '=':
            return element[1:]
        else:
            return "\"" + element + "\""
    else:
        return format(element, '.12f')

def df_to_list(df, include_headers = True):
    columns = list(df.columns.values)
    data = []
    column_length = 0
    for column in columns:
        column_data = df[column].tolist()

        if include_headers:
            column_data = [column] + column_data

        column_length = len(column_data)
        data = data + column_data
    return (data,column_length, len(columns))

def sheet(cell_range, data = None, headers = False, sheet_index = 0):

    # input data into sheet
    if data is not None:

        # convert numpy to array
        data_type_string = str(type(data))
        if data_type_string == "<class 'numpy.ndarray'>":
            data = data.tolist()

        if data_type_string == "<class 'pandas.core.series.Series'>":
            data = data.to_frame()
            data_type_string = str(type(data))

        if data_type_string == "<class 'pandas.core.frame.DataFrame'>":

            df_tuple = df_to_list(data, headers)
            data = df_tuple[0]
            column_length = df_tuple[1]
            column_count = df_tuple[2]

            # create cell_range
            if not has_number(cell_range):

                cellColumnLetter = cell_range
                cellColumnEndLetter = indexToLetters(letterToIndex(cellColumnLetter) + column_count - 1)
                cell_range = cellColumnLetter + "1:" + cellColumnEndLetter + str(column_length)
                
            else:

                cellColumnLetter = indexToLetters(getReferenceColumnIndex(cell_range))
                startRow = getReferenceRowIndex(cell_range)
                cellColumnEndLetter = indexToLetters(letterToIndex(cellColumnLetter) + column_count - 1)
                cell_range = cellColumnLetter + str(startRow) + ":" + cellColumnEndLetter + str(column_length + startRow - 1)
        

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

            arguments =  ['RANGE', 'SETLIST', cell_range, str(sheet_index)]

            # append list
            arguments = arguments + newList

            json_object = {'arguments':arguments}
            json_string = ''.join(['#PARSE#', json.dumps(json_object),'#ENDPARSE#'])
            real_print(json_string, flush=True, end='')

        else:
            data = convert_to_json_string(data)

            data = {'arguments': ['RANGE', 'SETSINGLE', cell_range, str(sheet_index), ''.join(["=",str(data)])]}
            data = ''.join(['#PARSE#', json.dumps(data),'#ENDPARSE#'])
            real_print(data, flush=True, end='')
    
    # get data from sheet
    else:
        #convert non-range to range for get operation
        if ":" not in cell_range:
            cell_range = ':'.join([cell_range, cell_range])

        # in blocking fashion get latest data of range from Go
        real_print("#DATA#" + str(sheet_index)+'!'+ cell_range + '#ENDPARSE#', end='', flush=True)
        getAndExecuteInputOnce()
        # if everything works, the exec command has filled sheet_data with the appropriate data
        # return data range as arrays
        column_references = cell_range_to_indexes(cell_range)

        result = []
        for column in column_references:
            column_data = []
            for reference in column:
                column_data.append(sheet_data[str(sheet_index)+'!'+reference])
            result.append(column_data)

        return pd.DataFrame(data=result).transpose()

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

                # onlyprint COMANDCOMPLETE when the input doesn't start with parseCall, 
                # since it's a special internal Python call 
                # which requires a single print between exec and return
                if not command_buffer.startswith("parseCall"):
                    real_print("#COMMANDCOMPLETE##ENDPARSE#", end='', flush = True)
            except:
                traceback.print_exc()
            command_buffer = ""
        else:
            command_buffer += code_input + "\n"

# testing
#sheet("A1:A2", [1,2])
# df = pd.DataFrame({'a':[1,2,3], 'b':[4,5,6]})
# sheet("A1:B2")

getAndExecuteInput()
