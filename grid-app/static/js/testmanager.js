(function(){
    
    function TestManager(app){

        var _this = this;
        
        this.app = app;

        this.tests = [];
        this.testStore = [];

        this.currentTestIndex = 0;
        this.currentTestCallback;

        this.testFailCount = 0;

        this.init = function(){
            // this.tests.push("testCopyValue");

            // by default, run all defined tests
            for(var testName in this.testStore){
                if(this.testStore.hasOwnProperty(testName)){
                    this.tests.push(testName);
                }
            }

            // override, for debugging
            this.tests = ['testCutWithReference'];
            window.test = function(){
                _this.runTests.apply(_this);
            };
        }

        this.runTests = function(){
            this.currentTestIndex = 0;
            this.testFailCount = 0;
            this.runTest();
        }

        this.reportTestingResult = function(){
            console.log("Testing result: "+(this.tests.length-this.testFailCount)+"/" + this.tests.length + " tests succeeded " + (this.testFailCount)+"/" + this.tests.length + " tests failed.");
        }
        this.runTest = function(){

            if(this.currentTestIndex < this.tests.length){
                var testName = this.tests[this.currentTestIndex];
                var currentTest = this.testStore[testName];
    
                this.currentTestCallback = function(){
    
                    var testResult = "Success";
    
                    for(var prop in currentTest.assert){
                        if(currentTest.assert.hasOwnProperty(prop)){
                            var splitted = prop.split("!");
                            var reference = splitted[1];
                            var sheet = parseInt(splitted[0]);
                            var expectedValue = currentTest.assert[prop];
    
                            var cellPosition = this.app.referenceToZeroIndexedArray(reference);
    
                            var actualValue = this.app.get(cellPosition, sheet);
    
                            if(expectedValue != actualValue){
                                console.log("Test #" + (this.currentTestIndex+1) + "/" + this.tests.length + " ("+testName+"): expected "
                                + prop + " as: " + expectedValue + " but got: " + actualValue);
    
                                testResult = "Failed";
                            }
    
                        }
                    }
    
                    if(testResult != "Success"){
                        this.testFailCount++;
                    }
    
                    console.log("Test #" + (this.currentTestIndex+1) + "/" + this.tests.length + " ("+testName+"): " + testResult);

                    this.currentTestIndex++;
                    this.runTest();
                }

                // execute test actions
                currentTest.run.apply(this);

                // execute test callback to call currentTestCallback
                this.app.wsManager.send({arguments: ["TESTCALLBACK-PING"]})

            }else{
                this.reportTestingResult();
            }
        }

        this.testStore["testCopyValue"] = (function(){

            var run = function(){
                this.app.wsManager.send({arguments: ["SET", "A1", "=100", "0"]})
                this.app.wsManager.send({arguments: ["COPY", "A1:A1", "0", "B1:B1", "0"]})
            }

            var assert = {
                "0!A1": "100",
                "0!B1": "100"
            }

            return {run: run, assert: assert};
        })();

        this.testStore["testCutValue"] = (function(){

            var run = function(){
                this.app.wsManager.send({arguments: ["SET", "A1", "=100", "0"]})
                this.app.wsManager.send({arguments: ["CUT", "A1:A1", "0", "B1:B1", "0"]})
            }

            var assert = {
                "0!A1": "",
                "0!B1": "100"
            }

            return {run: run, assert: assert};
        })();

        this.testStore["testCutWithReference"] = (function(){

            var run = function(){
                this.app.wsManager.send({arguments: ["SET", "A1", "=A2+1", "0"]})
                this.app.wsManager.send({arguments: ["SET", "A2", "=100", "0"]})
                this.app.wsManager.send({arguments: ["CUT", "A1:A2", "0", "B1:B2", "0"]})
            }

            var assert = {
                "0!A1": "",
                "0!A2": "",
                "0!B1": "101",
                "0!B2": "100"
            }

            return {run: run, assert: assert};
        })();

        this.testStore["testCutDifferentSheet"] = (function(){

            var run = function(){
                this.app.wsManager.send({arguments: ["SET", "A1", "=A2+1", "0"]})
                this.app.wsManager.send({arguments: ["SET", "A2", "=100", "0"]})
                this.app.wsManager.send({arguments: ["CUT", "A1:A2", "0", "A1:A2", "1"]})
            }

            var assert = {
                "0!A1": "",
                "0!A2": "",
                "1!A1": "101",
                "1!A2": "100"
            }

            return {run: run, assert: assert};
        })();

        this.testStore["testSumAndCut"] = (function(){

            var run = function(){
                this.app.wsManager.send({arguments: ["SET", "A1", "=1", "0"]})
                this.app.wsManager.send({arguments: ["SET", "A2", "=2", "0"]})
                this.app.wsManager.send({arguments: ["SET", "B1", "=SUM(A1:A2)", "0"]})
                this.app.wsManager.send({arguments: ["CUT", "A1:A2", "0", "A3:A4", "0"]})
            }

            var assert = {
                "0!A1": "",
                "0!A2": "",
                "0!A3": "1",
                "0!A4": "2",
                "0!B1": "3",
            }

            return {run: run, assert: assert};
        })();

    }

    window.TestManager = TestManager;
})()
