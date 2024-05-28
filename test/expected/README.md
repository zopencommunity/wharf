# Module Test File Format

The module test files are outputs of running Wharf and dumping the json output of the internal package and module changes (see commit e5d45ae). These are representations of what changes Wharf specifically made to fix the modules. These tests do not verify that Wharf has actually committed the changes and that they are correct, only that Wharf's decision making stays consistent.

## Generating Test Data

Currently the only way to generate the test data is to modify Wharf to output the data in some way that can be captured by the user and moved to a test file. This may change to a more standardized way in the future.

## Warning: Dependencies Break Tests

Due to how Wharf will choose updating dependencies over manually porting them, if possible, this may cause tests in the future to break - depending on if dependencies get updated to fix breaks in the future. This is because these tests currently expect identical decisions to be made as when the test data was recorded.

Until a more ideal fix has been made if any breaks occur during testing due to Wharf choosing to update over performing porting - please replace the test data with the new alterations <u>AND KEEP THE OLD JSON FILE</u> (please rename to it but keep them, we don't want to remove good test cases).

If you find a dependency requires porting that isn't included in the test suite as a standalone test please add it

